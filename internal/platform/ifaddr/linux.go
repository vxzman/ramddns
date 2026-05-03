//go:build linux

package ifaddr

import (
	"fmt"
	"time"

	stdnetlink "github.com/vishvananda/netlink"
)

// GetAvailableIPv6 returns IPv6 addresses from an interface using netlink.
func GetAvailableIPv6(interfaceName string) ([]IPv6Info, error) {
	link, err := stdnetlink.LinkByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to find interface %s: %w", interfaceName, err)
	}

	addrList, err := stdnetlink.AddrList(link, stdnetlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("failed to get address list for %s: %w", interfaceName, err)
	}

	var infos []IPv6Info
	for _, addr := range addrList {
		// Skip IPv4-mapped addresses
		if addr.IP.To4() != nil {
			continue
		}
		// Skip link-local addresses
		if addr.IP.IsLinkLocalUnicast() {
			continue
		}

		info := IPv6Info{
			IP:           addr.IP,
			PreferredLft: time.Duration(addr.PreferedLft) * time.Second,
			ValidLft:     time.Duration(addr.ValidLft) * time.Second,
		}
		PopulateInfo(&info)
		infos = append(infos, info)
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("no IPv6 address found on interface %s", interfaceName)
	}

	return infos, nil
}
