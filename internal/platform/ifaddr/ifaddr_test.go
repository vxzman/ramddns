package ifaddr

import (
	"net/netip"
	"testing"
	"time"
)

func mustIP(s string) netip.Addr {
	return netip.MustParseAddr(s)
}

func TestPopulateInfo(t *testing.T) {
	tests := []struct {
		name              string
		input             IPv6Info
		wantScope         string
		wantIsUniqueLocal bool
		wantIsDeprecated  bool
		wantIsCandidate   bool
	}{
		{
			name: "global_unicast",
			input: IPv6Info{
				IP:           mustIP("2001:db8::1"),
				PreferredLft: time.Hour * 24,
				ValidLft:     time.Hour * 48,
			},
			wantScope:         "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated:  false,
			wantIsCandidate:   true,
		},
		{
			name: "unique_local",
			input: IPv6Info{
				IP:           mustIP("fc00::1"),
				PreferredLft: time.Hour * 24,
				ValidLft:     time.Hour * 48,
			},
			wantScope:         "Unique Local (ULA)",
			wantIsUniqueLocal: true,
			wantIsDeprecated:  false,
			wantIsCandidate:   false,
		},
		{
			name: "unique_local_fd",
			input: IPv6Info{
				IP:           mustIP("fd00::1"),
				PreferredLft: time.Hour * 24,
				ValidLft:     time.Hour * 48,
			},
			wantScope:         "Unique Local (ULA)",
			wantIsUniqueLocal: true,
			wantIsDeprecated:  false,
			wantIsCandidate:   false,
		},
		{
			name: "link_local",
			input: IPv6Info{
				IP:           mustIP("fe80::1"),
				PreferredLft: time.Hour * 24,
				ValidLft:     time.Hour * 48,
			},
			wantScope:         "Link Local",
			wantIsUniqueLocal: false,
			wantIsDeprecated:  false,
			wantIsCandidate:   false,
		},
		{
			name: "deprecated_address",
			input: IPv6Info{
				IP:           mustIP("2001:db8::1"),
				PreferredLft: 0,
				ValidLft:     time.Hour * 24,
			},
			wantScope:         "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated:  true,
			wantIsCandidate:   false,
		},
		{
			name: "expired_address",
			input: IPv6Info{
				IP:           mustIP("2001:db8::1"),
				PreferredLft: 0,
				ValidLft:     0,
			},
			wantScope:         "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated:  false,
			wantIsCandidate:   false,
		},
		{
			name: "preferred_dynamic",
			input: IPv6Info{
				IP:           mustIP("2001:db8::1"),
				PreferredLft: time.Hour * 12,
				ValidLft:     time.Hour * 24,
			},
			wantScope:         "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated:  false,
			wantIsCandidate:   true,
		},
		{
			name: "preferred_static",
			input: IPv6Info{
				IP:           mustIP("2001:db8::1"),
				PreferredLft: time.Hour * 24 * 365,
				ValidLft:     time.Hour * 24 * 365,
			},
			wantScope:         "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated:  false,
			wantIsCandidate:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := tt.input
			PopulateInfo(&info)

			if info.Scope != tt.wantScope {
				t.Errorf("Scope = %v, want %v", info.Scope, tt.wantScope)
			}
			if info.IsUniqueLocal != tt.wantIsUniqueLocal {
				t.Errorf("IsUniqueLocal = %v, want %v", info.IsUniqueLocal, tt.wantIsUniqueLocal)
			}
			if info.IsDeprecated != tt.wantIsDeprecated {
				t.Errorf("IsDeprecated = %v, want %v", info.IsDeprecated, tt.wantIsDeprecated)
			}
			if info.IsCandidate != tt.wantIsCandidate {
				t.Errorf("IsCandidate = %v, want %v", info.IsCandidate, tt.wantIsCandidate)
			}
		})
	}
}

func TestIsPrivateOrLocalIP(t *testing.T) {
	tests := []struct {
		name string
		ip   netip.Addr
		want bool
	}{
		{"invalid", netip.Addr{}, true},
		{"link_local", mustIP("fe80::1"), true},
		{"unique_local_fc", mustIP("fc00::1"), true},
		{"unique_local_fd", mustIP("fd00::1"), true},
		{"loopback", mustIP("::1"), true},
		{"global_unicast", mustIP("2001:db8::1"), false},
		{"global_unicast_2", mustIP("240e:123:456::1"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPrivateOrLocalIP(tt.ip)
			if got != tt.want {
				t.Errorf("IsPrivateOrLocalIP(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestFilterValidAddresses(t *testing.T) {
	baseTime := time.Hour * 100

	tests := []struct {
		name      string
		input     []IPv6Info
		wantCount int
	}{
		{
			name: "all_valid",
			input: []IPv6Info{
				{
					IP:           mustIP("2001:db8::1"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
				{
					IP:           mustIP("2001:db8::2"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
			},
			wantCount: 2,
		},
		{
			name: "filter_nil_ip",
			input: []IPv6Info{
				{IP: netip.Addr{}, PreferredLft: baseTime, ValidLft: baseTime * 2},
				{
					IP:           mustIP("2001:db8::1"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_ipv4",
			input: []IPv6Info{
				{IP: mustIP("192.168.1.1"), PreferredLft: baseTime, ValidLft: baseTime * 2},
				{
					IP:           mustIP("2001:db8::1"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_link_local",
			input: []IPv6Info{
				{IP: mustIP("fe80::1"), PreferredLft: baseTime, ValidLft: baseTime * 2},
				{
					IP:           mustIP("2001:db8::1"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_expired",
			input: []IPv6Info{
				{IP: mustIP("2001:db8::1"), PreferredLft: baseTime, ValidLft: 0},
				{
					IP:           mustIP("2001:db8::2"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_deprecated",
			input: []IPv6Info{
				{IP: mustIP("2001:db8::1"), Scope: "Global Unicast", PreferredLft: 0, ValidLft: baseTime, IsDeprecated: true},
				{
					IP:           mustIP("2001:db8::2"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_unique_local",
			input: []IPv6Info{
				{IP: mustIP("fd00::1"), Scope: "Unique Local (ULA)", PreferredLft: baseTime, ValidLft: baseTime * 2},
				{
					IP:           mustIP("2001:db8::1"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
			},
			wantCount: 1,
		},
		{
			name: "all_filtered",
			input: []IPv6Info{
				{IP: mustIP("fe80::1"), PreferredLft: baseTime, ValidLft: baseTime * 2},
				{IP: mustIP("fd00::1"), PreferredLft: baseTime, ValidLft: baseTime * 2},
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterValidAddresses(tt.input)
			if len(got) != tt.wantCount {
				t.Errorf("FilterValidAddresses() returned %d addresses, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestSelectBestIPv6(t *testing.T) {
	baseTime := time.Hour * 100

	tests := []struct {
		name    string
		input   []IPv6Info
		wantIP  string
		wantErr bool
	}{
		{
			name: "single_valid",
			input: []IPv6Info{
				{
					IP:           mustIP("2001:db8::1"),
					Scope:        "Global Unicast",
					PreferredLft: baseTime,
					ValidLft:     baseTime * 2,
					IsCandidate:  true,
				},
			},
			wantIP:  "2001:db8::1",
			wantErr: false,
		},
		{
			name: "select_longest_preferred",
			input: []IPv6Info{
				{IP: mustIP("2001:db8::1"), Scope: "Global Unicast", PreferredLft: time.Hour * 50, ValidLft: time.Hour * 100, IsCandidate: true},
				{IP: mustIP("2001:db8::2"), Scope: "Global Unicast", PreferredLft: time.Hour * 200, ValidLft: time.Hour * 300, IsCandidate: true},
				{IP: mustIP("2001:db8::3"), Scope: "Global Unicast", PreferredLft: time.Hour * 100, ValidLft: time.Hour * 200, IsCandidate: true},
			},
			wantIP:  "2001:db8::2",
			wantErr: false,
		},
		{
			name: "no_valid_addresses",
			input: []IPv6Info{
				{IP: mustIP("fe80::1"), PreferredLft: baseTime, ValidLft: baseTime * 2},
				{IP: mustIP("fd00::1"), PreferredLft: baseTime, ValidLft: baseTime * 2},
			},
			wantIP:  "",
			wantErr: true,
		},
		{
			name:    "empty_input",
			input:   []IPv6Info{},
			wantIP:  "",
			wantErr: true,
		},
		{
			name: "tie_keeps_first",
			input: []IPv6Info{
				{IP: mustIP("2001:db8::1"), Scope: "Global Unicast", PreferredLft: baseTime, ValidLft: baseTime * 2, IsCandidate: true},
				{IP: mustIP("2001:db8::2"), Scope: "Global Unicast", PreferredLft: baseTime, ValidLft: baseTime * 2, IsCandidate: true},
			},
			wantIP:  "2001:db8::1",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectBestIPv6(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SelectBestIPv6() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantIP {
				t.Errorf("SelectBestIPv6() = %v, want %v", got, tt.wantIP)
			}
		})
	}
}
