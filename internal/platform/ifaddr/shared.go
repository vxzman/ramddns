package ifaddr

import (
	"errors"
	"net"
	"time"
)

// IPv6Info contains information about an IPv6 address
// 统一结构体，供各平台实现复用
type IPv6Info struct {
	IP             net.IP
	Scope          string
	AddressState   string
	PreferredLft   time.Duration
	ValidLft       time.Duration
	IsDeprecated   bool
	IsUniqueLocal  bool
	IsCandidate    bool // Whether it is a DDNS candidate
}

// populateInfo 填充 IPv6Info 的附加属性
// 所有平台获取 IP 后都应调用此函数
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

// IsPrivateOrLocalIP returns true for non-global addresses
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

// filterValidAddresses filters IPv6 candidates for DDNS
// 过滤规则：
// - 非 nil 且为 IPv6 地址
// - 非链路本地地址、非环回地址
// - 未过期 (ValidLft > 0)
// - 是全局单播地址且未废弃且非唯一本地地址
func filterValidAddresses(infos []IPv6Info) []IPv6Info {
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
		// 使用 IsCandidate 标志（由平台代码设置）或应用默认规则
		if info.IsCandidate || (info.Scope == "Global Unicast" && !info.IsDeprecated && !info.IsUniqueLocal) {
			out = append(out, info)
		}
	}
	return out
}

// SelectBestIPv6 selects the best IPv6 based on PreferredLft
// 选择生命周期最长的非私有 IP 地址
func SelectBestIPv6(infos []IPv6Info) (string, error) {
	candidates := filterValidAddresses(infos)

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
