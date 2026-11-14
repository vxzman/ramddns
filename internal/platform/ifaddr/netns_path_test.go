package ifaddr

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveNetNSPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "vpn", want: "/run/netns/vpn", ok: true},
		{in: "foo.bar", want: "/run/netns/foo.bar", ok: true},
		{in: "a-b_c", want: "/run/netns/a-b_c", ok: true},
		{in: "  vpn  ", want: "/run/netns/vpn", ok: true},
		{in: "/run/netns/vpn", want: "/run/netns/vpn", ok: true},
		{in: "/run/netns/vpn/../vpn", want: "/run/netns/vpn", ok: true},
		{in: "", ok: false},
		{in: ".", ok: false},
		{in: "..", ok: false},
		{in: "a..b", ok: false},
		{in: "../vpn", ok: false},
		{in: "has space", ok: false},
		{in: "/etc/passwd", ok: false},
		{in: "/run/netns", ok: false},
		{in: "/run/netns/", ok: false},
		{in: "/run/netns/../etc/passwd", ok: false},
	}
	longName := strings.Repeat("a", 64)
	tests = append(tests, struct {
		in   string
		want string
		ok   bool
	}{in: longName, want: "/run/netns/" + longName, ok: true})
	tests = append(tests, struct {
		in   string
		want string
		ok   bool
	}{in: longName + "b", ok: false})

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ResolveNetNSPath(tt.in)
			if tt.ok {
				if err != nil {
					t.Fatalf("ResolveNetNSPath(%q) error = %v", tt.in, err)
				}
				if got != tt.want {
					t.Fatalf("ResolveNetNSPath(%q) = %q, want %q", tt.in, got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ResolveNetNSPath(%q) = %q, want error", tt.in, got)
			}
		})
	}
}

func TestValidateGlobalIPv6(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "2001:db8::1", want: "2001:db8::1", ok: true},
		{in: "240e:123:456::1", want: "240e:123:456::1", ok: true},
		{in: "fe80::1", ok: false},
		{in: "fd00::1", ok: false},
		{in: "fc00::1", ok: false},
		{in: "::1", ok: false},
		{in: "192.0.2.1", ok: false},
		{in: "::ffff:192.0.2.1", ok: false},
		{in: "ff02::1", ok: false},
		{in: "", ok: false},
		{in: "not-an-ip", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ValidateGlobalIPv6(tt.in)
			if tt.ok {
				if err != nil {
					t.Fatalf("ValidateGlobalIPv6(%q) error = %v", tt.in, err)
				}
				if got.String() != tt.want {
					t.Fatalf("ValidateGlobalIPv6(%q) = %s, want %s", tt.in, got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateGlobalIPv6(%q) = %s, want error", tt.in, got)
			}
		})
	}
}

func TestParseProbeOutput(t *testing.T) {
	ok, err := ParseProbeOutput([]byte(`{"ip":"2001:db8::1","source":"interface","iface":"vpn-mv","netns":"vpn","preferred_lft":172594,"valid_lft":258994}`))
	if err != nil {
		t.Fatal(err)
	}
	if ok.IP != "2001:db8::1" || ok.Source != "interface" || ok.PreferredLft != 172594 || ok.ValidLft != 258994 {
		t.Fatalf("parsed = %+v", ok)
	}

	if _, err := ParseProbeOutput([]byte(`{"ip":"fd00::1","source":"interface","preferred_lft":1,"valid_lft":1}`)); err == nil {
		t.Fatal("ULA was accepted")
	}
	if _, err := ParseProbeOutput([]byte(`{`)); err == nil {
		t.Fatal("broken JSON was accepted")
	}
	if _, err := ParseProbeOutput([]byte(`{"ip":"2001:db8::1","source":"interface","preferred_lft":-1,"valid_lft":1}`)); err == nil {
		t.Fatal("negative lifetime was accepted")
	}
	if _, err := ParseProbeOutput([]byte(`{"ip":"2001:db8::1","source":"host","preferred_lft":1,"valid_lft":1}`)); err == nil {
		t.Fatal("bad source was accepted")
	}
}

func TestProbeExitMessage(t *testing.T) {
	if got := ProbeExitMessage("vpn", 10); got != "netns=vpn probe exit=10" {
		t.Fatalf("message = %q", got)
	}
}

func TestProbeErrorUnwrap(t *testing.T) {
	inner := errors.New("missing")
	err := &ProbeError{Code: ExitNetNSMissing, Err: inner}
	if !errors.Is(err, inner) {
		t.Fatal("errors.Is did not match the wrapped error")
	}
	if err.Error() == "" {
		t.Fatal("empty error text")
	}
}

func TestParseNameservers(t *testing.T) {
	text := "# comment\nnameserver 192.0.2.53\nnameserver 2001:db8::53\noptions ndots:0\n"
	got := ParseNameservers(text)
	if len(got) != 2 || got[0] != "192.0.2.53" || got[1] != "2001:db8::53" {
		t.Fatalf("nameservers = %#v", got)
	}
	if ParseNameservers("") != nil {
		t.Fatal("empty resolv.conf returned servers")
	}
}

func TestSplitList(t *testing.T) {
	got := SplitList(" https://a.example , https://b.example ")
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("SplitList = %#v", got)
	}
	if SplitList("  ") != nil {
		t.Fatal("blank list was not empty")
	}
}
