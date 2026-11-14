//go:build freebsd

package ifaddr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"golang.org/x/sys/unix"
)

// Netlink constants — FreeBSD values.
// FreeBSD 14+ netlink is wire-compatible with Linux netlink.
const (
	afNetlink    = 38
	netlinkRoute = 0

	// Netlink message types
	nlmsgDone  = 3
	nlmsgError = 2

	// Netlink message flags. NLM_F_DUMP is NLM_F_ROOT|NLM_F_MATCH (0x300).
	// 0x100 alone is only NLM_F_ROOT and is not a dump request.
	nlmFRequest = 0x01
	nlmFRoot    = 0x100
	nlmFMatch   = 0x200
	nlmFDump    = nlmFRoot | nlmFMatch

	// Route message types
	rtmNewAddr = 0x14
	rtmGetAddr = 0x16

	// Address attributes (same as Linux IFA_*)
	ifaAddress    = 1
	ifaLocal      = 2
	attrCacheinfo = 6 // IFA_CACHEINFO attribute type

	nd6InfiniteLifetime = 0xffffffff

	// Structure sizes on the wire
	sizeofNlMsghdr     = 16
	sizeofIfAddrmsg    = 8
	sizeofIfaCacheinfo = 16

	nlmsgAlignTo = 4

	// Same receive buffer as Gecko-ddns ip_getter_freebsd.cpp.
	netlinkRecvBuf = 8192
)

// --- Netlink wire structures (little-endian, same as Linux) ---

type nlMsghdr struct {
	Len   uint32 // total message length including header
	Type  uint16 // message type, e.g. RTM_NEWADDR
	Flags uint16 // NLM_F_*
	Seq   uint32 // sequence number
	Pid   uint32 // sender port ID
}

type ifAddrmsg struct {
	Family    uint8
	Prefixlen uint8
	Flags     uint8
	Scope     uint8
	Index     uint32
}

type ifaCacheinfo struct {
	Prefered uint32
	Valid    uint32
	Cstamp   uint32
	Tstamp   uint32
}

// nlmsgAlign rounds up to the nearest NLMSG_ALIGN boundary.
func nlmsgAlign(n int) int {
	return (n + nlmsgAlignTo - 1) &^ (nlmsgAlignTo - 1)
}

// parsedAddr holds attributes extracted from an RTM_NEWADDR message.
type parsedAddr struct {
	Family    uint8
	Prefixlen uint8
	Index     uint32
	Address   net.IP        // from IFA_ADDRESS
	Local     net.IP        // from IFA_LOCAL
	Cacheinfo *ifaCacheinfo // from IFA_CACHEINFO
}

// --- Public API ---

// GetAvailableIPv6 returns candidate IPv6 addresses and lifetimes from the named
// interface using the Netlink RTM_GETADDR dump — the same path ifconfig
// uses on FreeBSD 14+ (matching Gecko-ddns ip_getter_freebsd.cpp).
func GetAvailableIPv6(ifaceName string) ([]IPv6Info, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("interface not found or inaccessible: %w", err)
	}
	ifaceIndex := uint32(iface.Index)

	// 1. Open netlink socket with SOCK_CLOEXEC. Do not bind: the kernel
	// auto-binds on send, and nlmsg_pid stays 0. Binding to getpid() is a
	// different port and can fail with EADDRINUSE.
	fd, err := unix.Socket(afNetlink, unix.SOCK_RAW|unix.SOCK_CLOEXEC, netlinkRoute)
	if err != nil {
		return nil, fmt.Errorf("socket(AF_NETLINK) failed: %w", err)
	}
	defer unix.Close(fd)

	// 2. Set receive timeout (5 seconds), matching Gecko-ddns.
	tv := unix.Timeval{Sec: 5, Usec: 0}
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)

	// 3. Build RTM_GETADDR dump request.
	// Kernel filters by ifa_family and ifa_index (same path as FreeBSD ifconfig).
	nlh := nlMsghdr{
		Len:   sizeofNlMsghdr + sizeofIfAddrmsg,
		Type:  rtmGetAddr,
		Flags: nlmFRequest | nlmFDump,
		Seq:   1,
		Pid:   0,
	}
	ifam := ifAddrmsg{
		Family: unix.AF_INET6,
		Index:  ifaceIndex,
	}

	req := make([]byte, nlmsgAlign(int(nlh.Len)))
	putNlMsghdr(req, &nlh)
	putIfAddrmsg(req[sizeofNlMsghdr:], &ifam)

	// 4. send() with no destination. Same as Gecko-ddns.
	if err := unix.Sendto(fd, req, 0, nil); err != nil {
		return nil, fmt.Errorf("send(RTM_GETADDR) failed: %w", err)
	}

	// 5. Receive multi-part reply.
	buf := make([]byte, netlinkRecvBuf)
	var infos []IPv6Info
	dumpFinished := false

	for !dumpFinished {
		n, err := unix.Read(fd, buf)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				return nil, errors.New("netlink recv timeout")
			}
			return nil, fmt.Errorf("recv() failed: %w", err)
		}
		if n == 0 {
			return nil, errors.New("recv() returned empty")
		}

		offset := 0
		for offset+sizeofNlMsghdr <= int(n) {
			h := nlMsghdr{
				Len:   binary.LittleEndian.Uint32(buf[offset : offset+4]),
				Type:  binary.LittleEndian.Uint16(buf[offset+4 : offset+6]),
				Flags: binary.LittleEndian.Uint16(buf[offset+6 : offset+8]),
				Seq:   binary.LittleEndian.Uint32(buf[offset+8 : offset+12]),
				Pid:   binary.LittleEndian.Uint32(buf[offset+12 : offset+16]),
			}

			if h.Len < sizeofNlMsghdr || int(h.Len) > int(n)-offset {
				break
			}

			if h.Type == nlmsgDone {
				dumpFinished = true
				break
			}

			if h.Type == nlmsgError {
				payload := buf[offset+sizeofNlMsghdr : offset+int(h.Len)]
				if len(payload) >= 4 {
					errVal := int32(binary.LittleEndian.Uint32(payload[0:4]))
					if errVal != 0 {
						if errVal < 0 {
							errVal = -errVal
						}
						return nil, fmt.Errorf("netlink error: %w", unix.Errno(errVal))
					}
				}
				// ACK (error == 0), keep waiting for dump
			} else if h.Type == rtmNewAddr {
				payload := buf[offset+sizeofNlMsghdr : offset+int(h.Len)]
				if len(payload) >= sizeofIfAddrmsg {
					fam := payload[0]
					idx := binary.LittleEndian.Uint32(payload[4:8])
					if fam == unix.AF_INET6 && idx == ifaceIndex {
						var parsed parsedAddr
						parseAddrAttrs(payload[sizeofIfAddrmsg:], &parsed)

						// Prefer IFA_LOCAL for point-to-point links, fallback to IFA_ADDRESS
						chosen := parsed.Local
						if chosen == nil {
							chosen = parsed.Address
						}

						if chosen != nil && chosen.To4() == nil && !chosen.IsLinkLocalUnicast() && !chosen.IsLoopback() {
							if ip, ok := netip.AddrFromSlice(chosen); ok {
								ip = ip.Unmap()
								if ip.Is6() && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() {
									prefLft, validLft := cacheinfoLifetimes(parsed.Cacheinfo)
									info := IPv6Info{
										IP:           ip,
										PreferredLft: prefLft,
										ValidLft:     validLft,
									}
									PopulateInfo(&info)
									if info.IsCandidate {
										infos = append(infos, info)
									}
								}
							}
						}
					}
				}
			}

			step := nlmsgAlign(int(h.Len))
			offset += step
		}
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("no suitable IPv6 address on interface %s", ifaceName)
	}
	return infos, nil
}

// --- Serialization helpers ---

func putNlMsghdr(b []byte, h *nlMsghdr) {
	binary.LittleEndian.PutUint32(b[0:4], h.Len)
	binary.LittleEndian.PutUint16(b[4:6], h.Type)
	binary.LittleEndian.PutUint16(b[6:8], h.Flags)
	binary.LittleEndian.PutUint32(b[8:12], h.Seq)
	binary.LittleEndian.PutUint32(b[12:16], h.Pid)
}

func putIfAddrmsg(b []byte, m *ifAddrmsg) {
	b[0] = m.Family
	b[1] = m.Prefixlen
	b[2] = m.Flags
	b[3] = m.Scope
	binary.LittleEndian.PutUint32(b[4:8], m.Index)
}

// --- Parsing ---

// parseAddrAttrs walks the rtattr list after ifAddrmsg.
func parseAddrAttrs(b []byte, a *parsedAddr) {
	for len(b) >= 4 {
		attrLen := binary.LittleEndian.Uint16(b[0:2])
		attrType := binary.LittleEndian.Uint16(b[2:4])
		if attrLen < 4 || int(attrLen) > len(b) {
			break
		}

		data := b[4:attrLen]

		switch attrType {
		case ifaAddress:
			if len(data) >= 16 {
				ip := make(net.IP, 16)
				copy(ip, data[:16])
				a.Address = ip
			}
		case ifaLocal:
			if len(data) >= 16 {
				ip := make(net.IP, 16)
				copy(ip, data[:16])
				a.Local = ip
			}
		case attrCacheinfo:
			if len(data) >= sizeofIfaCacheinfo {
				a.Cacheinfo = &ifaCacheinfo{
					Prefered: binary.LittleEndian.Uint32(data[0:4]),
					Valid:    binary.LittleEndian.Uint32(data[4:8]),
					Cstamp:   binary.LittleEndian.Uint32(data[8:12]),
					Tstamp:   binary.LittleEndian.Uint32(data[12:16]),
				}
			}
		}

		// Advance to next attribute (RTA_ALIGN). Stop if padding would run
		// past this message; the kernel may omit trailing pad bytes.
		advance := nlmsgAlign(int(attrLen))
		if advance > len(b) {
			break
		}
		b = b[advance:]
	}
}

// cacheinfoLifetimes maps FreeBSD ifa_cacheinfo to durations.
// Missing cacheinfo and 0xffffffff are infinite. 0 is expired.
// Same rules as Gecko-ddns ip_getter_freebsd.cpp.
func cacheinfoLifetimes(ci *ifaCacheinfo) (preferred, valid time.Duration) {
	preferred = InfiniteLifetime
	valid = InfiniteLifetime
	if ci == nil {
		return preferred, valid
	}
	if ci.Prefered != 0 && ci.Prefered != nd6InfiniteLifetime {
		preferred = time.Duration(ci.Prefered) * time.Second
	} else if ci.Prefered == 0 {
		preferred = 0
	}
	if ci.Valid != 0 && ci.Valid != nd6InfiniteLifetime {
		valid = time.Duration(ci.Valid) * time.Second
	} else if ci.Valid == 0 {
		valid = 0
	}
	return preferred, valid
}
