package ifaddr

import (
	"net"
	"time"
)

// IPv6Info contains information about an IPv6 address.
// This is the unified structure used across all platforms.
type IPv6Info struct {
	IP            net.IP
	Scope         string
	AddressState  string
	PreferredLft  time.Duration
	ValidLft      time.Duration
	IsDeprecated  bool
	IsUniqueLocal bool
	IsCandidate   bool // Whether it is a DDNS candidate
}
