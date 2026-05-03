package ifaddr

import (
	"errors"
	"net"
	"time"
)

// PopulateInfo fills in derived fields of IPv6Info based on IP and lifetimes.
// All platform implementations should call this after obtaining raw address data.
func PopulateInfo(info *IPv6Info) {
	if info.IP == nil {
		return
	}

	ipBytes := info.IP
	info.IsUniqueLocal = ipBytes[0] == 0xfc || ipBytes[0] == 0xfd

	if info.IP.IsLinkLocalUnicast() {
		info.Scope = "Link Local"
	} else if info.IsUniqueLocal {
		info.Scope = "Unique Local (ULA)"
	} else {
		info.Scope = "Global Unicast"
	}

	info.IsDeprecated = info.PreferredLft.Seconds() <= 0 && info.ValidLft.Seconds() > 0

	if info.ValidLft.Seconds() == 0 {
		info.AddressState = "Expired"
	} else if info.IsDeprecated {
		info.AddressState = "Deprecated"
	} else if info.PreferredLft.Seconds() < info.ValidLft.Seconds() {
		info.AddressState = "Preferred/Dynamic"
	} else {
		info.AddressState = "Preferred/Static"
	}

	info.IsCandidate = info.Scope == "Global Unicast" && !info.IsDeprecated && !info.IsUniqueLocal && info.ValidLft.Seconds() > 0
}

// IsPrivateOrLocalIP returns true for non-global addresses.
func IsPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLinkLocalUnicast() {
		return true
	}
	if ip[0] == 0xfc || ip[0] == 0xfd {
		return true
	}
	if ip.IsLoopback() {
		return true
	}
	return false
}

// FilterValidAddresses filters IPv6 addresses suitable for DDNS.
// Filtering rules:
//   - Non-nil and IPv6 address
//   - Not link-local, not loopback
//   - Not expired (ValidLft > 0)
//   - Is global unicast and not deprecated and not ULA
func FilterValidAddresses(infos []IPv6Info) []IPv6Info {
	var out []IPv6Info
	for _, info := range infos {
		if info.IP == nil {
			continue
		}
		if info.IP.To4() != nil {
			continue
		}
		if info.IP.IsLinkLocalUnicast() || info.IP.IsLoopback() {
			continue
		}
		if info.ValidLft.Seconds() == 0 {
			continue
		}
		if info.IsCandidate || (info.Scope == "Global Unicast" && !info.IsDeprecated && !info.IsUniqueLocal) {
			out = append(out, info)
		}
	}
	return out
}

// SelectBestIPv6 selects the best IPv6 address based on PreferredLft.
// Returns the address with the longest preferred lifetime.
func SelectBestIPv6(infos []IPv6Info) (string, error) {
	candidates := FilterValidAddresses(infos)

	if len(candidates) == 0 {
		return "", errors.New("no suitable DDNS Candidate (Global Unicast, not deprecated) found")
	}

	var bestCandidate IPv6Info
	maxPreferredLft := time.Duration(0)
	for _, info := range candidates {
		if info.PreferredLft > maxPreferredLft {
			maxPreferredLft = info.PreferredLft
			bestCandidate = info
		}
	}

	return bestCandidate.IP.String(), nil
}
