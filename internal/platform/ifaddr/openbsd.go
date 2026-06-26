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

// in6_ifreq layout: 16-byte name + union { ..., icmp6_ifstat (272 bytes), ... }
// sizeof(struct in6_ifreq) = 288 on OpenBSD (all architectures).
const (
	ifrNameLen  = 16  // IFNAMSIZ
	ifrUnionOff = 16
	ifrSize     = 288 // sizeof(struct in6_ifreq)
)

// nd6InfiniteLifetime is the u_int32 sentinel for "forever".
const nd6InfiniteLifetime = 0xffffffff

// siocGifAlifetimeIn6 returns the SIOCGIFALIFETIME_IN6 ioctl command.
// _IOWR('i', 81, struct in6_ifreq) where sizeof(struct in6_ifreq) = 288.
func siocGifAlifetimeIn6() uintptr {
	const (
		iocInOut = 0xC0000000
		group    = 'i'
		num      = 81
		size     = ifrSize
	)
	return iocInOut | ((size & 0x1fff) << 16) | (group << 8) | num
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

	ioctlCmd := siocGifAlifetimeIn6()
	now := time.Now().Unix()
	var infos []IPv6Info

	log.Info("openbsd: found %d addrs on %s, ioctl=0x%x", len(addrs), ifaceName, ioctlCmd)

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

		var ifr [ifrSize]byte
		copy(ifr[:ifrNameLen], ifaceName)

		sin6 := (*unix.RawSockaddrInet6)(unsafe.Pointer(&ifr[ifrUnionOff]))
		sin6.Len = unix.SizeofSockaddrInet6
		sin6.Family = unix.AF_INET6
		copy(sin6.Addr[:], ip.To16())

		_, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			uintptr(fd),
			ioctlCmd,
			uintptr(unsafe.Pointer(&ifr[0])),
		)
		if errno != 0 {
			log.Info("openbsd: addr[%d]=%s ioctl failed: %v", i, ip, errno)
			continue
		}

		lt := (*in6Addrlifetime)(unsafe.Pointer(&ifr[ifrUnionOff]))

		var pltime uint32 = nd6InfiniteLifetime
		var vltime uint32 = nd6InfiniteLifetime

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
