package ifaddr

import (
	"cmp"
	"errors"
	"net/netip"
	"slices"
	"time"
)

// PopulateInfo fills in derived fields of IPv6Info based on IP and lifetimes.
// All platform implementations should call this after obtaining raw address data.
func PopulateInfo(info *IPv6Info) {
	if info == nil || !info.IP.IsValid() {
		return
	}

	addr := info.IP.Unmap()
	info.IP = addr
	info.IsUniqueLocal = addr.Is6() && addr.IsPrivate()

	if addr.IsLinkLocalUnicast() {
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

	info.IsCandidate = addr.Is6() && info.Scope == "Global Unicast" && !info.IsDeprecated && !info.IsUniqueLocal && info.ValidLft.Seconds() > 0
}

// IsPrivateOrLocalIP returns true for non-global addresses.
func IsPrivateOrLocalIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return true
	}
	ip = ip.Unmap()
	if !ip.Is6() || ip.Is4() {
		return true
	}
	return ip.IsLinkLocalUnicast() || ip.IsPrivate() || ip.IsLoopback()
}

// BestBy returns the item with the greatest key. Ties keep the first item.
func BestBy[T any](items []T, key func(T) time.Duration) (T, bool) {
	if len(items) == 0 {
		var zero T
		return zero, false
	}
	best := slices.MaxFunc(items, func(a, b T) int {
		return cmp.Compare(key(a), key(b))
	})
	return best, true
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
		addr := info.IP.Unmap()
		if !addr.IsValid() || !addr.Is6() || addr.Is4() {
			continue
		}
		if addr.IsLinkLocalUnicast() || addr.IsLoopback() {
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

// SelectBestInfo selects the candidate with the longest preferred lifetime.
// The filter rules are the same ones SelectBestIPv6 has always used.
func SelectBestInfo(infos []IPv6Info) (IPv6Info, error) {
	candidates := FilterValidAddresses(infos)
	best, ok := BestBy(candidates, func(info IPv6Info) time.Duration {
		return info.PreferredLft
	})
	if !ok || !best.IP.IsValid() {
		return IPv6Info{}, errors.New("no suitable DDNS Candidate (Global Unicast, not deprecated) found")
	}
	return best, nil
}

// SelectBestIPv6 selects the best IPv6 address based on PreferredLft.
// Returns the address with the longest preferred lifetime.
func SelectBestIPv6(infos []IPv6Info) (string, error) {
	best, err := SelectBestInfo(infos)
	if err != nil {
		return "", err
	}
	return best.IP.String(), nil
}
