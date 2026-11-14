package ifaddr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"ramddns/internal/log"
)

const (
	// apiTimeout is the per-request HTTP client timeout.
	apiTimeout = 15 * time.Second

	// apiMaxRetries is the number of retries for a single API call.
	apiMaxRetries = 3

	// apiTotalTimeout is the maximum time to wait for all APIs combined.
	apiTotalTimeout = 15 * time.Second
)

// parseAPIAddress extracts the first line of an echo-API body and requires IPv6.
func parseAPIAddress(body string) (netip.Addr, error) {
	cleaned := strings.ReplaceAll(body, "\r", "")
	var ipStr string
	for _, line := range strings.Split(cleaned, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ipStr = line
			break
		}
	}
	if ipStr == "" {
		return netip.Addr{}, errors.New("empty response from API")
	}
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("API returned non-IPv6 address: %q", ipStr)
	}
	addr = addr.Unmap()
	if !addr.Is6() || addr.Is4() {
		return netip.Addr{}, fmt.Errorf("API returned non-IPv6 address: %q", ipStr)
	}
	return addr, nil
}

func infosFromAPI(addr netip.Addr) []IPv6Info {
	info := IPv6Info{
		IP:           addr,
		PreferredLft: InfiniteLifetime,
		ValidLft:     InfiniteLifetime,
	}
	PopulateInfo(&info)
	return []IPv6Info{info}
}

// fetchIPFromURL queries one HTTP API. client may be nil.
func fetchIPFromURL(ctx context.Context, client *http.Client, rawURL string) (netip.Addr, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = &http.Client{Timeout: apiTimeout}
	}

	var lastErr error
	for attempt := 0; attempt <= apiMaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return netip.Addr{}, err
		}
		if attempt > 0 {
			delay := time.Duration(1<<(attempt-1)) * time.Second
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return netip.Addr{}, ctx.Err()
			case <-timer.C:
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "curl/8.0.0")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}

		addr, err := parseAPIAddress(string(body))
		if err != nil {
			log.Warning("API returned invalid IPv6", "url", rawURL, "err", err)
			lastErr = err
			continue
		}
		return addr, nil
	}

	return netip.Addr{}, fmt.Errorf("HTTP request failed after %d retries: %w", apiMaxRetries, lastErr)
}

// GetIPv6FromAPIs queries HTTP APIs concurrently and returns the first IPv6.
// It starts goroutines. Do not call it after setns; the probe child uses
// GetIPv6FromAPIsSequential instead.
func GetIPv6FromAPIs(ctx context.Context, urls []string, quiet bool) ([]IPv6Info, error) {
	if len(urls) == 0 {
		return nil, errors.New("no API URLs configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, apiTotalTimeout)
	defer cancel()

	client := &http.Client{Timeout: apiTimeout}
	type apiResult struct {
		addr netip.Addr
		err  error
	}
	resultCh := make(chan apiResult, len(urls))
	var wg sync.WaitGroup
	for _, u := range urls {
		if !quiet {
			log.Info("Querying API", "url", u)
		}
		wg.Add(1)
		go func(rawURL string) {
			defer wg.Done()
			addr, err := fetchIPFromURL(ctx, client, rawURL)
			resultCh <- apiResult{addr: addr, err: err}
		}(u)
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var errs []error
	for {
		select {
		case <-ctx.Done():
			if !quiet {
				log.Error("API query timeout", "timeout", apiTotalTimeout, "err", ctx.Err())
			}
			return nil, fmt.Errorf("API query timeout: %w", ctx.Err())
		case result, ok := <-resultCh:
			if !ok {
				if !quiet {
					log.Error("All APIs failed", "count", len(urls))
				}
				return nil, fmt.Errorf("all API requests failed: %w", errors.Join(errs...))
			}
			if result.err == nil {
				cancel()
				if !quiet {
					log.Info("API succeeded", "ip", result.addr.String())
				}
				return infosFromAPI(result.addr), nil
			}
			if !quiet {
				log.Error("API failed", "err", result.err)
			}
			errs = append(errs, result.err)
		}
	}
}

// GetIPv6FromAPIsSequential queries APIs on the calling goroutine.
// The probe child calls this after setns. client carries the netns dialer;
// a nil client uses the default transport.
func GetIPv6FromAPIsSequential(ctx context.Context, urls []string, client *http.Client) ([]IPv6Info, error) {
	if len(urls) == 0 {
		return nil, errors.New("no API URLs configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, apiTotalTimeout)
	defer cancel()

	var errs []error
	for _, rawURL := range urls {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("API query timeout: %w", err)
		}
		log.Info("Querying API", "url", rawURL)
		addr, err := fetchIPFromURL(ctx, client, rawURL)
		if err == nil {
			log.Info("API succeeded", "ip", addr.String())
			return infosFromAPI(addr), nil
		}
		log.Error("API failed", "url", rawURL, "err", err)
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("all API requests failed: %w", errors.Join(errs...))
}
