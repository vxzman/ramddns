//go:build darwin

package ifaddr

/*
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <net/if.h>
#include <netinet/in.h>
#include <netinet6/in6_var.h>
#include <arpa/inet.h>
#include <ifaddrs.h>
#include <time.h>
#include <errno.h>

#define ND6_INFINITE_LIFETIME 0xffffffffU

// 返回值：0=成功有地址，1=接口不存在，2=无全局地址，-1=其他错误
// addresses_buf: 调用方分配的缓冲区，用于存放完整的 JSON 数组 "[{...},{...}]"
// max_len: 缓冲区大小
int get_ipv6info_darwin(const char *ifname, char *addresses_buf, size_t max_len, int *error_code) {
	*error_code = 0;

	if (if_nametoindex(ifname) == 0) {
		*error_code = 1;
		return 1;
	}

	struct ifaddrs *ifap = NULL;
	if (getifaddrs(&ifap) == -1) {
		*error_code = -1;
		return -1;
	}

	int s = socket(AF_INET6, SOCK_DGRAM, 0);
	if (s == -1) {
		freeifaddrs(ifap);
		*error_code = -1;
		return -1;
	}

	time_t now = time(NULL);

	int count = 0;
	char *ptr = addresses_buf;
	size_t remain = max_len;

	ptr += snprintf(ptr, remain, "[");
	remain = max_len - (ptr - addresses_buf);

	struct ifaddrs *ifa;
	for (ifa = ifap; ifa != NULL; ifa = ifa->ifa_next) {
		if (ifa->ifa_addr == NULL || strcmp(ifa->ifa_name, ifname) != 0 ||
			ifa->ifa_addr->sa_family != AF_INET6)
			continue;

		struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *)ifa->ifa_addr;
		char addr_str[INET6_ADDRSTRLEN];
		if (inet_ntop(AF_INET6, &sin6->sin6_addr, addr_str, sizeof(addr_str)) == NULL)
			continue;

		struct in6_ifreq ifr6;
		memset(&ifr6, 0, sizeof(ifr6));
		strncpy(ifr6.ifr_name, ifname, IFNAMSIZ-1);
		ifr6.ifr_addr = *sin6;

		// macOS uses SIOCGIFALIFETIME_IN6 similar to FreeBSD
		if (ioctl(s, SIOCGIFALIFETIME_IN6, &ifr6) == -1)
			continue;

		struct in6_addrlifetime lt = ifr6.ifr_ifru.ifru_lifetime;

		// Convert to pltime/vltime format
		// macOS may have slightly different behavior, handle with care
		unsigned int pltime = (lt.ia6t_preferred != (time_t)-1 && lt.ia6t_preferred > now)
			? (unsigned int)(lt.ia6t_preferred - now) : ND6_INFINITE_LIFETIME;
		unsigned int vltime = (lt.ia6t_expire != (time_t)-1 && lt.ia6t_expire > now)
			? (unsigned int)(lt.ia6t_expire - now) : ND6_INFINITE_LIFETIME;

		if (count > 0) {
			ptr += snprintf(ptr, remain, ",");
			remain = max_len - (ptr - addresses_buf);
		}

		ptr += snprintf(ptr, remain,
			"{\"addr\":\"%s\",\"pltime\":%u,\"vltime\":%u}",
			addr_str, pltime, vltime);
		remain = max_len - (ptr - addresses_buf);
		count++;
	}

	ptr += snprintf(ptr, remain, "]");

	close(s);
	freeifaddrs(ifap);

	if (count == 0) {
		*error_code = 2;
		return 2;
	}

	return 0;
}
*/
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
	"unsafe"
)

type ioctlAddrInfo struct {
	Addr   string `json:"addr"`
	Pltime uint32 `json:"pltime"` // Preferred lifetime in seconds
	Vltime uint32 `json:"vltime"` // Valid lifetime in seconds
}

// GetAvailableIPv6 uses ioctl to get IPv6 info on macOS (Darwin)
// Note: This is experimental support. macOS API behavior may differ from FreeBSD.
func GetAvailableIPv6(ifaceName string) ([]IPv6Info, error) {
	cIfname := C.CString(ifaceName)
	defer C.free(unsafe.Pointer(cIfname))

	const bufSize = 4096
	buf := make([]byte, bufSize)
	cBuf := (*C.char)(unsafe.Pointer(&buf[0]))

	var errcode C.int
	ret := C.get_ipv6info_darwin(cIfname, cBuf, C.size_t(bufSize), &errcode)

	jsonStr := C.GoString(cBuf)

	switch ret {
	case 0:
		var addrs []ioctlAddrInfo
		if err := json.Unmarshal([]byte(jsonStr), &addrs); err != nil {
			return nil, err
		}

		var infos []IPv6Info
		for _, a := range addrs {
			ip := net.ParseIP(a.Addr)
			info := IPv6Info{
				IP:           ip,
				PreferredLft: time.Duration(a.Pltime) * time.Second,
				ValidLft:     time.Duration(a.Vltime) * time.Second,
			}
			PopulateInfo(&info)
			infos = append(infos, info)
		}

		if len(infos) == 0 {
			return nil, errors.New("no valid global IPv6 addresses after parsing")
		}
		return infos, nil

	case 1:
		return nil, errors.New("interface not found or inaccessible")

	case 2:
		return nil, errors.New("no global IPv6 address found on interface")

	default:
		return nil, fmt.Errorf("unexpected error from cgo ioctl (code=%d)", ret)
	}
}
