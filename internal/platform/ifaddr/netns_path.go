package ifaddr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ExitOK           = 0
	ExitNetNSMissing = 10
	ExitPermission   = 11
	ExitNoAddress    = 12
	ExitNoIface      = 13
	ExitProbeParse   = 14
)

// ProbeResult is the single JSON object written by the __probe child.
// Lifetimes are seconds.
type ProbeResult struct {
	IP           string `json:"ip"`
	Source       string `json:"source"`
	Iface        string `json:"iface"`
	NetNS        string `json:"netns"`
	PreferredLft int64  `json:"preferred_lft"`
	ValidLft     int64  `json:"valid_lft"`
}

// ProbeError is a probe failure with a stable exit code.
type ProbeError struct {
	Code int
	Err  error
}

func (e *ProbeError) Error() string {
	if e == nil {
		return "probe failed"
	}
	if e.Err == nil {
		return fmt.Sprintf("probe exit %d", e.Code)
	}
	return fmt.Sprintf("probe exit %d: %v", e.Code, e.Err)
}

func (e *ProbeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ProbeExitMessage is the parent log line required for every probe result.
func ProbeExitMessage(ns string, code int) string {
	return fmt.Sprintf("netns=%s probe exit=%d", ns, code)
}

var (
	errInvalidNetNSName = errors.New("invalid netns name")

	netnsNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

// ResolveNetNSPath accepts a name or an absolute path under /run/netns.
// Names containing ".." are rejected even though the character class allows dots.
func ResolveNetNSPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errInvalidNetNSName
	}
	if strings.HasPrefix(name, "/") {
		clean := filepath.Clean(name)
		if clean == "/run/netns" || !strings.HasPrefix(clean, "/run/netns/") {
			return "", fmt.Errorf("%w: %s", errInvalidNetNSName, name)
		}
		return clean, nil
	}
	if name == "." || name == ".." || strings.Contains(name, "..") || !netnsNameRe.MatchString(name) {
		return "", fmt.Errorf("%w: %q", errInvalidNetNSName, name)
	}
	return filepath.Join("/run/netns", name), nil
}

// ValidateGlobalIPv6 accepts a global unicast IPv6 address.
// ULA, link-local, loopback, and IPv4-mapped addresses are rejected.
func ValidateGlobalIPv6(s string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid ip %q: %w", s, err)
	}
	addr = addr.Unmap()
	switch {
	case !addr.Is6():
		return netip.Addr{}, fmt.Errorf("not ipv6: %s", addr)
	case addr.IsLinkLocalUnicast(), addr.IsLoopback():
		return netip.Addr{}, fmt.Errorf("non-global scope: %s", addr)
	case addr.IsPrivate():
		return netip.Addr{}, fmt.Errorf("ula address: %s", addr)
	case !addr.IsGlobalUnicast():
		return netip.Addr{}, fmt.Errorf("not global unicast: %s", addr)
	}
	return addr, nil
}

// ParseProbeOutput checks the child JSON and the address it carries.
func ParseProbeOutput(out []byte) (ProbeResult, error) {
	var result ProbeResult
	if err := json.Unmarshal(out, &result); err != nil {
		return ProbeResult{}, fmt.Errorf("parse probe output: %w", err)
	}
	addr, err := ValidateGlobalIPv6(result.IP)
	if err != nil {
		return ProbeResult{}, err
	}
	result.IP = addr.String()
	if result.PreferredLft < 0 || result.ValidLft < 0 {
		return ProbeResult{}, errors.New("negative lifetime")
	}
	if result.Source != "interface" && result.Source != "api" {
		return ProbeResult{}, fmt.Errorf("invalid source %q", result.Source)
	}
	return result, nil
}

// ParseNameservers reads nameserver addresses from resolv.conf text.
func ParseNameservers(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "nameserver") {
			out = append(out, fields[1])
		}
	}
	return out
}

// NetNSNameservers reads the resolv.conf that belongs to a netns name.
// The Go resolver runs on other threads, which do not see a per-thread bind mount.
func NetNSNameservers(ns string) []string {
	candidates := []string{
		filepath.Join("/etc/netns", ns, "resolv.conf"),
		"/usr/local/etc/resolv.conf",
	}
	for _, src := range candidates {
		text, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		servers := ParseNameservers(string(text))
		if len(servers) > 0 {
			return servers
		}
	}
	return nil
}

// SplitList splits a comma-separated flag value.
func SplitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
