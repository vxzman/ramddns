package ifaddr

import (
	"errors"
	"net/netip"
	"time"
)

// InfiniteLifetime is a large duration that acts as "forever" for static
// addresses and API-obtained addresses. 100 years — safe within int64 nanosecond range.
const InfiniteLifetime = time.Hour * 24 * 365 * 100

// ErrInterfaceNotFound is returned when the named interface does not exist.
var ErrInterfaceNotFound = errors.New("interface not found")

// IPv6Info contains information about an IPv6 address.
// This is the unified structure used across all platforms.
type IPv6Info struct {
	IP            netip.Addr
	Scope         string
	AddressState  string
	PreferredLft  time.Duration
	ValidLft      time.Duration
	IsDeprecated  bool
	IsUniqueLocal bool
	IsCandidate   bool // Whether it is a DDNS candidate
}
