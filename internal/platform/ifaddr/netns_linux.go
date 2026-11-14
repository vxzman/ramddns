//go:build linux

package ifaddr

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
	"ramddns/internal/log"
)

// RunProbe is the __probe child. It is the only place that calls netns.Set.
// Address discovery stays on this thread. HTTP dials setns their own thread
// before creating a socket, because net/http resolves and connects elsewhere.
func RunProbe(netnsName, iface string, urls []string) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	path, err := ResolveNetNSPath(netnsName)
	if err != nil {
		log.Error("invalid netns", "netns", netnsName, "err", err)
		return ExitNetNSMissing
	}

	handle, err := netns.GetFromPath(path)
	if err != nil {
		log.Error("open netns", "path", path, "err", err)
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) || errors.Is(err, os.ErrPermission) {
			log.Error("netns permission denied; need root or CAP_SYS_ADMIN", "path", path)
			return ExitPermission
		}
		return ExitNetNSMissing
	}
	defer handle.Close()

	if err := netns.Set(handle); err != nil {
		log.Error("netns permission denied; need root or CAP_SYS_ADMIN", "path", path, "err", err)
		return ExitPermission
	}

	nsBase := filepath.Base(path)
	bindNetNSResolv(nsBase)
	nameservers := NetNSNameservers(nsBase)

	infos, err := GetAvailableIPv6(iface)
	source := "interface"
	ifaceMissing := err != nil && errors.Is(err, ErrInterfaceNotFound)
	if err != nil {
		log.Warning("interface lookup failed", "iface", iface, "err", err)
		infos, err = GetIPv6FromAPIsSequential(context.Background(), urls, newNetNSHTTPClient(handle, nameservers))
		source = "api"
		if err != nil {
			log.Error("no usable address in netns", "netns", netnsName, "iface", iface, "err", err)
			if ifaceMissing {
				return ExitNoIface
			}
			return ExitNoAddress
		}
	}

	best, err := SelectBestInfo(infos)
	if err != nil {
		log.Error("no usable address in netns", "netns", netnsName, "iface", iface, "err", err)
		if ifaceMissing {
			return ExitNoIface
		}
		return ExitNoAddress
	}

	result := ProbeResult{
		IP:           best.IP.String(),
		Source:       source,
		Iface:        iface,
		NetNS:        netnsName,
		PreferredLft: int64(best.PreferredLft / time.Second),
		ValidLft:     int64(best.ValidLft / time.Second),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(result); err != nil {
		log.Error("encode probe result", "err", err)
		return ExitNoAddress
	}
	return ExitOK
}

// bindNetNSResolv bind-mounts a resolv.conf into the child's mount namespace.
// A failed unshare must not bind-mount, or the host resolv.conf would change.
func bindNetNSResolv(ns string) {
	if err := unix.Unshare(unix.CLONE_NEWNS); err != nil {
		log.Warning("unshare mount namespace failed", "err", err)
		return
	}
	if err := unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		log.Warning("make root private failed", "err", err)
		return
	}

	candidates := []string{
		filepath.Join("/etc/netns", ns, "resolv.conf"),
		"/usr/local/etc/resolv.conf",
	}
	for _, src := range candidates {
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := unix.Mount(src, "/etc/resolv.conf", "", unix.MS_BIND, ""); err != nil {
			log.Warning("bind resolv.conf failed", "src", src, "err", err)
		}
		return
	}
}

func newNetNSHTTPClient(ns netns.NsHandle, nameservers []string) *http.Client {
	var dnsSeq atomic.Uint32
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialInNetNS(ctx, ns, nameservers, &dnsSeq, network, addr)
		},
		ForceAttemptHTTP2:     false,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	return &http.Client{Timeout: apiTimeout, Transport: transport}
}

// dialInNetNS enters the probe netns on the dialing thread, then connects.
// net/http and the Go resolver start their own goroutines; each of those
// threads must setns before it creates a socket. unshare(CLONE_NEWNS) does
// not cover those threads, so DNS dials the nameserver from the netns
// resolv.conf instead of the host address the resolver parsed.
func dialInNetNS(ctx context.Context, ns netns.NsHandle, nameservers []string, dnsSeq *atomic.Uint32, network, addr string) (net.Conn, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := netns.Set(ns); err != nil {
		return nil, fmt.Errorf("setns before dial: %w", err)
	}
	dialer := net.Dialer{
		Timeout: 10 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				if err := netns.Set(ns); err != nil {
					return nil, fmt.Errorf("setns before dns: %w", err)
				}
				var dnsDialer net.Dialer
				dnsDialer.Timeout = 5 * time.Second
				return dnsDialer.DialContext(ctx, network, rewriteDNSAddress(address, nameservers, dnsSeq))
			},
		},
	}
	return dialer.DialContext(ctx, network, addr)
}

func rewriteDNSAddress(address string, nameservers []string, dnsSeq *atomic.Uint32) string {
	if len(nameservers) == 0 || dnsSeq == nil {
		return address
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return address
	}
	idx := int(dnsSeq.Add(1)-1) % len(nameservers)
	return net.JoinHostPort(nameservers[idx], port)
}
