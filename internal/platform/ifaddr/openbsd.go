//go:build openbsd

package ifaddr

import (
	"errors"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
	"ramddns/internal/log"
)

// in6_addrlifetime mirrors struct in6_addrlifetime from <netinet6/in6_var.h>.
// Layout on OpenBSD:
//
//	time_t   ia6t_expire    (8 bytes on LP64, 4 on ILP32)
//	time_t   ia6t_preferred (8 bytes on LP64, 4 on ILP32)
//	u_int32_t ia6t_vltime   (4 bytes)
//	u_int32_t ia6t_pltime   (4 bytes)
type in6Addrlifetime struct {
	Expire    int64
	Preferred int64
	Vltime    uint32
	Pltime    uint32
}

// in6_ifreq layout: 16-byte name + max(28, sizeof(in6_addrlifetime)) union.
const (
	ifrNameLen  = 16 // IFNAMSIZ
	ifrUnionOff = 16
	ifrSizePad  = 48 // with 8-byte tail alignment (standard LP64)
)

// nd6InfiniteLifetime is the u_int32 sentinel for "forever".
const nd6InfiniteLifetime = 0xffffffff

// siocGifAlifetimeIn6 returns candidate SIOCGIFALIFETIME_IN6 values.
// sizeof(struct in6_ifreq) may be 48 (with tail padding) or 44 (without).
// Both sizes are tried since the exact value depends on the platform ABI.
func siocGifAlifetimeIn6() []uintptr {
	const (
		iocInOut = 0xC0000000
		group    = 'i'
		num      = 81
	)
	mk := func(size uintptr) uintptr {
		return iocInOut | ((size & 0x1fff) << 16) | (group << 8) | num
	}
	return []uintptr{mk(48), mk(44)}
}

// GetAvailableIPv6 returns IPv6 addresses and lifetimes from the named
// interface using getifaddrs (via net.Interface.Addrs) to enumerate
// addresses and ioctl(SIOCGIFALIFETIME_IN6) for each to retrieve
// preferred / valid lifetimes.
func GetAvailableIPv6(ifaceName string) ([]IPv6Info, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, errors.New("interface not found or inaccessible")
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, errors.New("failed to enumerate interface addresses")
	}

	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_DGRAM, 0)
	if err != nil {
		return nil, errors.New("system error while querying interface addresses")
	}
	defer unix.Close(fd)

	ioctlCmds := siocGifAlifetimeIn6()
	now := time.Now().Unix()
	var infos []IPv6Info

	log.Info("openbsd: found %d addrs on %s", len(addrs), ifaceName)

	for i, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			log.Info("openbsd: addr[%d] not IPNet, skipping", i)
			continue
		}
		ip := ipnet.IP
		if ip.To4() != nil {
			log.Info("openbsd: addr[%d]=%s IPv4, skip", i, ip)
			continue
		}
		if ip.IsLinkLocalUnicast() {
			log.Info("openbsd: addr[%d]=%s link-local, skip", i, ip)
			continue
		}

		log.Info("openbsd: addr[%d]=%s trying ioctls %x/%x...",
			i, ip, ioctlCmds[0], ioctlCmds[1])

		var found bool
		var pltime, vltime uint32

		for _, cmd := range ioctlCmds {
			var ifr [ifrSizePad]byte
			copy(ifr[:ifrNameLen], ifaceName)

			sin6 := (*unix.RawSockaddrInet6)(unsafe.Pointer(&ifr[ifrUnionOff]))
			sin6.Len = unix.SizeofSockaddrInet6
			sin6.Family = unix.AF_INET6
			copy(sin6.Addr[:], ip.To16())

			_, _, errno := unix.Syscall(
				unix.SYS_IOCTL,
				uintptr(fd),
				cmd,
				uintptr(unsafe.Pointer(&ifr[0])),
			)
			if errno != 0 {
				log.Info("openbsd: addr[%d] ioctl 0x%x: %v", i, cmd, errno)
				continue
			}

			lt := (*in6Addrlifetime)(unsafe.Pointer(&ifr[ifrUnionOff]))
			pltime = nd6InfiniteLifetime
			vltime = nd6InfiniteLifetime

			// time_t value of 0 means "no lifetime info" (permanent address).
			if lt.Preferred != 0 {
				remaining := lt.Preferred - now
				if remaining > 0 {
					pltime = uint32(remaining)
				} else {
					pltime = 0
				}
			}
			if lt.Expire != 0 {
				remaining := lt.Expire - now
				if remaining > 0 {
					vltime = uint32(remaining)
				} else {
					vltime = 0
				}
			}
			found = true
			break
		}

		if !found {
			continue
		}

		log.Info("openbsd: addr[%d]=%s pltime=%d vltime=%d", i, ip, pltime, vltime)

		if pltime == nd6InfiniteLifetime {
			pltime = uint32((365 * 10 * 24 * time.Hour).Seconds())
		}
		if vltime == nd6InfiniteLifetime {
			vltime = uint32((365 * 10 * 24 * time.Hour).Seconds())
		}

		info := IPv6Info{
			IP:           ip,
			PreferredLft: time.Duration(pltime) * time.Second,
			ValidLft:     time.Duration(vltime) * time.Second,
		}
		PopulateInfo(&info)
		infos = append(infos, info)
	}

	if len(infos) == 0 {
		return nil, errors.New("no global IPv6 address found on interface")
	}

	return infos, nil
}
