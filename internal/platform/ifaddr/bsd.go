//go:build freebsd || openbsd || darwin

package ifaddr

/*
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/ioctl.h>
#include <net/if.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <ifaddrs.h>
#include <time.h>
#include <errno.h>

#if defined(__FreeBSD__) || defined(__OpenBSD__) || defined(__APPLE__)
#include <netinet6/in6_var.h>
#endif

#ifndef ND6_INFINITE_LIFETIME
#define ND6_INFINITE_LIFETIME 0xffffffffU
#endif

typedef struct {
	char addr[INET6_ADDRSTRLEN];
	unsigned int pltime;
	unsigned int vltime;
} bsd_addr_info;

int query_interface(const char *ifname, bsd_addr_info *results, int max_results, int *error_code) {
	*error_code = 0;

	if (if_nametoindex(ifname) == 0) {
		*error_code = 1;
		return 0;
	}

	struct ifaddrs *ifap = NULL;
	if (getifaddrs(&ifap) == -1) {
		*error_code = -1;
		return 0;
	}

	int s = socket(AF_INET6, SOCK_DGRAM, 0);
	if (s == -1) {
		freeifaddrs(ifap);
		*error_code = -1;
		return 0;
	}

	time_t now = time(NULL);
	int count = 0;

	for (struct ifaddrs *ifa = ifap; ifa != NULL && count < max_results; ifa = ifa->ifa_next) {
		if (ifa->ifa_addr == NULL ||
			strcmp(ifa->ifa_name, ifname) != 0 ||
			ifa->ifa_addr->sa_family != AF_INET6) {
			continue;
		}

		struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *)ifa->ifa_addr;
		char addr_str[INET6_ADDRSTRLEN];
		if (inet_ntop(AF_INET6, &sin6->sin6_addr, addr_str, sizeof(addr_str)) == NULL) {
			continue;
		}

		unsigned int pltime = ND6_INFINITE_LIFETIME;
		unsigned int vltime = ND6_INFINITE_LIFETIME;

#if defined(__FreeBSD__) || defined(__APPLE__)
		struct in6_ifreq ifr6;
		memset(&ifr6, 0, sizeof(ifr6));
		strncpy(ifr6.ifr_name, ifname, IFNAMSIZ - 1);
		ifr6.ifr_addr = *sin6;

		if (ioctl(s, SIOCGIFALIFETIME_IN6, &ifr6) == 0) {
			struct in6_addrlifetime lt = ifr6.ifr_ifru.ifru_lifetime;
#ifdef __APPLE__
			if (lt.ia6t_preferred != (time_t)-1 && lt.ia6t_preferred > now)
				pltime = (unsigned int)(lt.ia6t_preferred - now);
			if (lt.ia6t_expire != (time_t)-1 && lt.ia6t_expire > now)
				vltime = (unsigned int)(lt.ia6t_expire - now);
#else
			if (lt.ia6t_preferred != (time_t)-1)
				pltime = (unsigned int)(lt.ia6t_preferred - now);
			if (lt.ia6t_expire != (time_t)-1)
				vltime = (unsigned int)(lt.ia6t_expire - now);
#endif
		}
#elif defined(__OpenBSD__)
		struct in6_ifreq ifr6;
		memset(&ifr6, 0, sizeof(ifr6));
		strlcpy(ifr6.ifr_name, ifname, IFNAMSIZ);
		ifr6.ifr_addr = *sin6;

		if (ioctl(s, SIOCGIFALIFETIME_IN6, &ifr6) == 0) {
			struct in6_addrlifetime lt = ifr6.ifr_ifru.ifru_lifetime;
			if (lt.ia6t_preferred != (time_t)-1)
				pltime = (unsigned int)(lt.ia6t_preferred - now);
			if (lt.ia6t_expire != (time_t)-1)
				vltime = (unsigned int)(lt.ia6t_expire - now);
		}
#endif

		strncpy(results[count].addr, addr_str, INET6_ADDRSTRLEN - 1);
		results[count].addr[INET6_ADDRSTRLEN - 1] = '\0';
		results[count].pltime = pltime;
		results[count].vltime = vltime;
		count++;
	}

	close(s);
	freeifaddrs(ifap);

	if (count == 0 && *error_code == 0) {
		*error_code = 2;
	}

	return count;
}
*/
import "C"

import (
	"errors"
	"net"
	"time"
	"unsafe"
)

// bsdAddrInfo mirrors C.bsd_addr_info in Go.
type bsdAddrInfo struct {
	Addr   string
	Pltime uint32
	Vltime uint32
}

// GetAvailableIPv6 returns IPv6 addresses from an interface on BSD systems.
// Uses getifaddrs() + ioctl() to query the kernel, analogous to Linux netlink.
func GetAvailableIPv6(ifaceName string) ([]IPv6Info, error) {
	cIfname := C.CString(ifaceName)
	defer C.free(unsafe.Pointer(cIfname))

	const maxAddrs = 64
	cResults := make([]C.bsd_addr_info, maxAddrs)

	var errcode C.int
	count := C.query_interface(cIfname, &cResults[0], C.int(maxAddrs), &errcode)

	switch errcode {
	case 0:
		if count == 0 {
			return nil, errors.New("no valid IPv6 addresses found")
		}

		infos := make([]IPv6Info, 0, int(count))
		for i := 0; i < int(count); i++ {
			ip := net.ParseIP(C.GoString(&cResults[i].addr[0]))
			if ip == nil {
				continue
			}

			info := IPv6Info{
				IP:           ip,
				PreferredLft: time.Duration(cResults[i].pltime) * time.Second,
				ValidLft:     time.Duration(cResults[i].vltime) * time.Second,
			}
			PopulateInfo(&info)
			infos = append(infos, info)
		}

		if len(infos) == 0 {
			return nil, errors.New("no valid IPv6 addresses found")
		}
		return infos, nil

	case 1:
		return nil, errors.New("interface not found or inaccessible")
	case 2:
		return nil, errors.New("no global IPv6 address found on interface")
	default:
		return nil, errors.New("system error while querying interface addresses")
	}
}
