//go:build openbsd

package ifaddr

import (
	"errors"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// in6_addrlifetime mirrors struct in6_addrlifetime from <netinet6/in6_var.h>.
type in6Addrlifetime struct {
	Expire    int64
	Preferred int64
	Vltime    uint32
	Pltime    uint32
}

// sizeof(struct in6_ifreq) = 288 on OpenBSD.
// The union includes icmp6_ifstat (272 bytes), which dominates.
const (
	ifrNameLen  = 16  // IFNAMSIZ
	ifrUnionOff = 16
	ifrSize     = 288 // sizeof(struct in6_ifreq)
)

// nd6InfiniteLifetime is the u_int32 sentinel for "forever".
const nd6InfiniteLifetime = 0xffffffff

// siocGifAlifetimeIn6 returns the SIOCGIFALIFETIME_IN6 ioctl command.
// _IOWR('i', 81, struct in6_ifreq) where sizeof = 288.
func siocGifAlifetimeIn6() uintptr {
	const (
		iocInOut = 0xC0000000
		group    = 'i'
		num      = 81
	)
	return iocInOut | ((ifrSize & 0x1fff) << 16) | (group << 8) | num
}

// GetAvailableIPv6 returns IPv6 addresses and lifetimes from the named
// interface using net.Interface.Addrs to enumerate addresses and
// ioctl(SIOCGIFALIFETIME_IN6) for lifetimes.
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

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP
		if ip.To4() != nil || ip.IsLinkLocalUnicast() {
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
