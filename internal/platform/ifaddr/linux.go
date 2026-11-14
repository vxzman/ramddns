//go:build linux

package ifaddr

import (
	"errors"
	"fmt"
	"net/netip"
	"time"

	stdnetlink "github.com/vishvananda/netlink"
)

// GetAvailableIPv6 returns candidate IPv6 addresses from an interface using netlink.
func GetAvailableIPv6(interfaceName string) ([]IPv6Info, error) {
	link, err := stdnetlink.LinkByName(interfaceName)
	if err != nil {
		var missing stdnetlink.LinkNotFoundError
		if errors.As(err, &missing) {
			return nil, &interfaceMissingError{name: interfaceName, err: err}
		}
		return nil, fmt.Errorf("failed to find interface %s: %w", interfaceName, err)
	}

	addrList, err := stdnetlink.AddrList(link, stdnetlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("failed to get address list for %s: %w", interfaceName, err)
	}

	var infos []IPv6Info
	for _, addr := range addrList {
		ip, ok := netip.AddrFromSlice(addr.IP)
		if !ok {
			continue
		}
		ip = ip.Unmap()
		// Skip IPv4, IPv4-mapped, link-local, and loopback.
		if !ip.Is6() || ip.Is4() || ip.IsLinkLocalUnicast() || ip.IsLoopback() {
			continue
		}

		prefLft := InfiniteLifetime
		if addr.PreferedLft != 0xFFFFFFFF && addr.PreferedLft > 0 {
			prefLft = time.Duration(addr.PreferedLft) * time.Second
		} else if addr.PreferedLft == 0 {
			prefLft = 0
		}

		validLft := InfiniteLifetime
		if addr.ValidLft != 0xFFFFFFFF && addr.ValidLft > 0 {
			validLft = time.Duration(addr.ValidLft) * time.Second
		} else if addr.ValidLft == 0 {
			validLft = 0
		}

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

	if len(infos) == 0 {
		return nil, fmt.Errorf("no suitable IPv6 address on interface %s", interfaceName)
	}

	return infos, nil
}

// interfaceMissingError is one log line and still matches ErrInterfaceNotFound.
type interfaceMissingError struct {
	name string
	err  error
}

func (e *interfaceMissingError) Error() string {
	return fmt.Sprintf("failed to find interface %s: %s", e.name, e.err.Error())
}

func (e *interfaceMissingError) Unwrap() []error {
	return []error{ErrInterfaceNotFound, e.err}
}
