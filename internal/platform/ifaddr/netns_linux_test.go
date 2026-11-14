//go:build linux

package ifaddr

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__probe" {
		os.Exit(probeChildArgs(os.Args[2:]))
	}
	os.Exit(m.Run())
}

func probeChildArgs(args []string) int {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ns := fs.String("netns", "", "")
	iface := fs.String("iface", "", "")
	urls := fs.String("fallback-urls", "", "")
	if err := fs.Parse(args); err != nil {
		return ExitProbeParse
	}
	return RunProbe(*ns, *iface, SplitList(*urls))
}

func TestInterfaceNotFoundSentinel(t *testing.T) {
	_, err := GetAvailableIPv6("ramddns-no-such-iface0")
	if !errors.Is(err, ErrInterfaceNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestProbeMissingNetNS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := ProbeNetNS(ctx, "ramddns-no-such-ns", "lo", nil)
	var probeErr *ProbeError
	if !errors.As(err, &probeErr) {
		t.Fatalf("error = %v", err)
	}
	if probeErr.Code != ExitNetNSMissing {
		t.Fatalf("code = %d, want %d (%v)", probeErr.Code, ExitNetNSMissing, err)
	}
}

func TestProbeInvalidNameExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := ProbeNetNS(ctx, "../vpn", "lo", nil)
	var probeErr *ProbeError
	if !errors.As(err, &probeErr) {
		t.Fatalf("error = %v", err)
	}
	if probeErr.Code != ExitNetNSMissing {
		t.Fatalf("code = %d, want %d (%v)", probeErr.Code, ExitNetNSMissing, err)
	}
	if !strings.Contains(err.Error(), "invalid netns") && !strings.Contains(err.Error(), "probe exit") {
		t.Fatalf("error = %v", err)
	}
}

func TestProbeNetNS(t *testing.T) {
	if _, err := os.Stat("/run/netns/vpn"); err != nil {
		t.Skip("netns vpn not available")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := ProbeNetNS(ctx, "vpn", "ramddns-no-such-iface0", nil)
	var probeErr *ProbeError
	if !errors.As(err, &probeErr) {
		t.Fatalf("error = %v", err)
	}
	if probeErr.Code != ExitNoIface && probeErr.Code != ExitNoAddress {
		t.Fatalf("code = %d, want %d or %d (%v)", probeErr.Code, ExitNoIface, ExitNoAddress, err)
	}
}
