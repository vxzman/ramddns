package ifaddr

import (
	"net"
	"testing"
	"time"
)

func TestPopulateInfo(t *testing.T) {
	tests := []struct {
		name           string
		input          IPv6Info
		wantScope      string
		wantIsUniqueLocal bool
		wantIsDeprecated bool
		wantIsCandidate bool
	}{
		{
			name: "global_unicast",
			input: IPv6Info{
				IP:           net.ParseIP("2001:db8::1"),
				PreferredLft: time.Hour * 24,
				ValidLft:     time.Hour * 48,
			},
			wantScope:      "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated: false,
			wantIsCandidate: true,
		},
		{
			name: "unique_local",
			input: IPv6Info{
				IP:           net.ParseIP("fc00::1"),
				PreferredLft: time.Hour * 24,
				ValidLft:     time.Hour * 48,
			},
			wantScope:      "Unique Local (ULA)",
			wantIsUniqueLocal: true,
			wantIsDeprecated: false,
			wantIsCandidate: false,
		},
		{
			name: "unique_local_fd",
			input: IPv6Info{
				IP:           net.ParseIP("fd00::1"),
				PreferredLft: time.Hour * 24,
				ValidLft:     time.Hour * 48,
			},
			wantScope:      "Unique Local (ULA)",
			wantIsUniqueLocal: true,
			wantIsDeprecated: false,
			wantIsCandidate: false,
		},
		{
			name: "link_local",
			input: IPv6Info{
				IP:           net.ParseIP("fe80::1"),
				PreferredLft: time.Hour * 24,
				ValidLft:     time.Hour * 48,
			},
			wantScope:      "Link Local",
			wantIsUniqueLocal: false,
			wantIsDeprecated: false,
			wantIsCandidate: false,
		},
		{
			name: "deprecated_address",
			input: IPv6Info{
				IP:           net.ParseIP("2001:db8::1"),
				PreferredLft: 0,
				ValidLft:     time.Hour * 24,
			},
			wantScope:      "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated: true,
			wantIsCandidate: false,
		},
		{
			name: "expired_address",
			input: IPv6Info{
				IP:           net.ParseIP("2001:db8::1"),
				PreferredLft: 0,
				ValidLft:     0,
			},
			wantScope:      "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated: false, // Expired (ValidLft=0) is different from Deprecated
			wantIsCandidate: false,  // Not a candidate because ValidLft=0
		},
		{
			name: "preferred_dynamic",
			input: IPv6Info{
				IP:           net.ParseIP("2001:db8::1"),
				PreferredLft: time.Hour * 12,
				ValidLft:     time.Hour * 24,
			},
			wantScope:      "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated: false,
			wantIsCandidate: true,
		},
		{
			name: "preferred_static",
			input: IPv6Info{
				IP:           net.ParseIP("2001:db8::1"),
				PreferredLft: time.Hour * 24 * 365,
				ValidLft:     time.Hour * 24 * 365,
			},
			wantScope:      "Global Unicast",
			wantIsUniqueLocal: false,
			wantIsDeprecated: false,
			wantIsCandidate: true,
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
		ip   net.IP
		want bool
	}{
		{"nil_ip", nil, true},
		{"link_local", net.ParseIP("fe80::1"), true},
		{"unique_local_fc", net.ParseIP("fc00::1"), true},
		{"unique_local_fd", net.ParseIP("fd00::1"), true},
		{"loopback", net.ParseIP("::1"), true},
		{"global_unicast", net.ParseIP("2001:db8::1"), false},
		{"global_unicast_2", net.ParseIP("240e:123:456::1"), false},
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
					IP: net.ParseIP("2001:db8::1"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
				{
					IP: net.ParseIP("2001:db8::2"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
			},
			wantCount: 2,
		},
		{
			name: "filter_nil_ip",
			input: []IPv6Info{
				{
					IP: nil,
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
				},
				{
					IP: net.ParseIP("2001:db8::1"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_ipv4",
			input: []IPv6Info{
				{
					IP: net.ParseIP("192.168.1.1"),
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
				},
				{
					IP: net.ParseIP("2001:db8::1"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_link_local",
			input: []IPv6Info{
				{
					IP: net.ParseIP("fe80::1"),
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
				},
				{
					IP: net.ParseIP("2001:db8::1"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_expired",
			input: []IPv6Info{
				{
					IP: net.ParseIP("2001:db8::1"),
					PreferredLft: baseTime,
					ValidLft: 0, // expired
				},
				{
					IP: net.ParseIP("2001:db8::2"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_deprecated",
			input: []IPv6Info{
				{
					IP: net.ParseIP("2001:db8::1"),
					Scope: "Global Unicast",
					PreferredLft: 0,
					ValidLft: baseTime,
					IsDeprecated: true,
				},
				{
					IP: net.ParseIP("2001:db8::2"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
			},
			wantCount: 1,
		},
		{
			name: "filter_unique_local",
			input: []IPv6Info{
				{
					IP: net.ParseIP("fd00::1"),
					Scope: "Unique Local (ULA)",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
				},
				{
					IP: net.ParseIP("2001:db8::1"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
			},
			wantCount: 1,
		},
		{
			name: "all_filtered",
			input: []IPv6Info{
				{
					IP: net.ParseIP("fe80::1"),
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
				},
				{
					IP: net.ParseIP("fd00::1"),
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
				},
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterValidAddresses(tt.input)
			if len(got) != tt.wantCount {
				t.Errorf("filterValidAddresses() returned %d addresses, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestSelectBestIPv6(t *testing.T) {
	baseTime := time.Hour * 100

	tests := []struct {
		name      string
		input     []IPv6Info
		wantIP    string
		wantErr   bool
	}{
		{
			name: "single_valid",
			input: []IPv6Info{
				{
					IP: net.ParseIP("2001:db8::1"),
					Scope: "Global Unicast",
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
					IsCandidate: true,
				},
			},
			wantIP:  "2001:db8::1",
			wantErr: false,
		},
		{
			name: "select_longest_preferred",
			input: []IPv6Info{
				{
					IP: net.ParseIP("2001:db8::1"),
					Scope: "Global Unicast",
					PreferredLft: time.Hour * 50,
					ValidLft: time.Hour * 100,
					IsCandidate: true,
				},
				{
					IP: net.ParseIP("2001:db8::2"),
					Scope: "Global Unicast",
					PreferredLft: time.Hour * 200,
					ValidLft: time.Hour * 300,
					IsCandidate: true,
				},
				{
					IP: net.ParseIP("2001:db8::3"),
					Scope: "Global Unicast",
					PreferredLft: time.Hour * 100,
					ValidLft: time.Hour * 200,
					IsCandidate: true,
				},
			},
			wantIP:  "2001:db8::2",
			wantErr: false,
		},
		{
			name: "no_valid_addresses",
			input: []IPv6Info{
				{
					IP: net.ParseIP("fe80::1"),
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
				},
				{
					IP: net.ParseIP("fd00::1"),
					PreferredLft: baseTime,
					ValidLft: baseTime * 2,
				},
			},
			wantIP:  "",
			wantErr: true,
		},
		{
			name: "empty_input",
			input: []IPv6Info{},
			wantIP: "",
			wantErr: true,
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
