package ifaddr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"ramddns/internal/log"
)

// ProbeNetNS re-execs this program as __probe. The parent never calls netns.Set.
func ProbeNetNS(ctx context.Context, ns, iface string, urls []string) (ProbeResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return ProbeResult{}, &ProbeError{Code: ExitProbeParse, Err: fmt.Errorf("resolve executable: %w", err)}
	}

	args := []string{"__probe", "--netns", ns, "--iface", iface}
	if len(urls) > 0 {
		args = append(args, "--fallback-urls", strings.Join(urls, ","))
	}

	cmd := exec.CommandContext(ctx, exe, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	code := probeExitCode(err)
	stderrText := strings.TrimSpace(stderr.String())
	if len(stderrText) > 4096 {
		stderrText = stderrText[:4096]
	}

	if err != nil {
		log.Info(ProbeExitMessage(ns, code), "netns", ns, "exit", code)
		if stderrText != "" {
			log.Warning("probe stderr", "netns", ns, "stderr", stderrText)
		}
		cause := err
		if stderrText != "" {
			cause = fmt.Errorf("%w: %s", err, stderrText)
		}
		return ProbeResult{}, &ProbeError{Code: code, Err: cause}
	}

	result, parseErr := ParseProbeOutput(out)
	if parseErr != nil {
		log.Info(ProbeExitMessage(ns, ExitProbeParse), "netns", ns, "exit", ExitProbeParse)
		if stderrText != "" {
			log.Warning("probe stderr", "netns", ns, "stderr", stderrText)
		}
		return ProbeResult{}, &ProbeError{Code: ExitProbeParse, Err: fmt.Errorf("parse probe output: %w", parseErr)}
	}
	log.Info(ProbeExitMessage(ns, ExitOK), "netns", ns, "exit", ExitOK)
	if stderrText != "" {
		log.Warning("probe stderr", "netns", ns, "stderr", stderrText)
	}
	return result, nil
}

func probeExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			return ExitProbeParse
		}
		return code
	}
	return ExitProbeParse
}
