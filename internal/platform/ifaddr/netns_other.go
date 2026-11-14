//go:build !linux

package ifaddr

import (
	"fmt"
	"os"
)

// RunProbe is the non-Linux __probe child. It does not call netns.Set.
func RunProbe(netnsName, iface string, urls []string) int {
	_, _ = netnsName, iface
	_ = urls
	fmt.Fprintln(os.Stderr, "netns is only supported on Linux")
	return 1
}
