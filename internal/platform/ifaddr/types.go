package ifaddr

import (
	"net"
	"time"
)

// InfiniteLifetime is a large duration that acts as "forever" for static
// addresses and API-obtained addresses. 100 years — safe within int64 nanosecond range.
const InfiniteLifetime = time.Hour * 24 * 365 * 100

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
