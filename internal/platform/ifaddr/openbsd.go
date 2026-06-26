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
// sizeof(struct in6_ifreq) is 48 on LP64 (tail-padded to 8-byte alignment),
// and 44 on ILP32 (no tail padding needed).
const (
	ifrNameLen   = 16 // IFNAMSIZ
	ifrUnionOff  = 16
	ifrSizeLP64  = 48
	ifrSizeILP32 = 44
)

// nd6InfiniteLifetime is the u_int32 sentinel for "forever".
const nd6InfiniteLifetime = 0xffffffff

// siocGifAlifetimeIn6 computes the SIOCGIFALIFETIME_IN6 ioctl command at
// runtime, because sizeof(struct in6_ifreq) varies between LP64 (48) and
// ILP32 (44).  The macro is _IOWR('i', 81, sizeof(struct in6_ifreq)).
func siocGifAlifetimeIn6() uintptr {
	const (
		iocInOut = 0xC0000000
		group    = 'i'
		num      = 81
	)
	size := uintptr(ifrSizeILP32)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		size = ifrSizeLP64
	}
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

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP
		if ip.To4() != nil {
			continue
		}
		// Skip link-local — lifetime queries are not meaningful for them.
		if ip.IsLinkLocalUnicast() {
			continue
		}

		// Build in6_ifreq as a raw byte buffer (max size for both ABIs).
		var ifr [ifrSizeLP64]byte
		copy(ifr[:ifrNameLen], ifaceName)

		// Write sockaddr_in6 into the union at offset 16.
		sin6 := (*unix.RawSockaddrInet6)(unsafe.Pointer(&ifr[ifrUnionOff]))
		sin6.Len = unix.SizeofSockaddrInet6
		sin6.Family = unix.AF_INET6
		copy(sin6.Addr[:], ip.To16())

		// Query lifetime via ioctl — the kernel overwrites the union
		// with struct in6_addrlifetime.
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

		// Map sentinel to a generous duration so the address is
		// considered Preferred/Static by PopulateInfo.
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
