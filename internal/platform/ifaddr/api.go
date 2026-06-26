package ifaddr

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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

	// infiniteLifetime is a large duration that acts as "forever" for
	// API-obtained addresses (no real lifetime info available).
	// 100 years — safe within int64 nanosecond range.
	infiniteLifetime = time.Hour * 24 * 365 * 100
)

// fetchIPFromURL queries a single HTTP API and returns a normalized IPv6
// string.  Returns empty string + error on failure.
// Matches Gecko-ddns fetch_ip_from_url() behavior.
func fetchIPFromURL(url string) (string, error) {
	client := &http.Client{Timeout: apiTimeout}

	var lastErr error
	for attempt := 0; attempt <= apiMaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<(attempt-1)) * time.Second
			time.Sleep(delay)
		}

		resp, err := client.Get(url)
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

		// Strip \r (handle \r\n line endings) and extract first non-empty line.
		// Matches Gecko-ddns fetch_ip_from_url().
		cleaned := strings.ReplaceAll(string(body), "\r", "")
		lines := strings.Split(cleaned, "\n")
		ipStr := ""
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				ipStr = line
				break
			}
		}

		if ipStr == "" {
			lastErr = errors.New("empty response from API")
			continue
		}

		// Validate that it is a well-formed IPv6 address.
		ip := net.ParseIP(ipStr)
		if ip == nil || ip.To4() != nil {
			log.Warning("API %s returned invalid IPv6: '%s'", url, ipStr)
			lastErr = fmt.Errorf("API returned non-IPv6 address: '%s'", ipStr)
			continue
		}

		// Normalize to canonical form (inet_ntop equivalent).
		return ip.String(), nil
	}

	return "", fmt.Errorf("HTTP request failed after %d retries: %v", apiMaxRetries, lastErr)
}

// GetIPv6FromAPIs queries multiple HTTP APIs concurrently and returns the
// first successful IPv6 result.  Matches Gecko-ddns get_from_apis() behavior.
func GetIPv6FromAPIs(urls []string, quiet bool) ([]IPv6Info, error) {
	if len(urls) == 0 {
		return nil, errors.New("no API URLs configured")
	}

	type apiResult struct {
		ip  string
		err error
	}

	var wg sync.WaitGroup
	resultCh := make(chan apiResult, len(urls))
	doneCh := make(chan struct{})
	var once sync.Once

	for _, u := range urls {
		if !quiet {
			log.Info("Querying API: %s", u)
		}
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			ip, err := fetchIPFromURL(url)

			select {
			case <-doneCh:
				// Another goroutine already succeeded; discard result.
				return
			default:
			}

			resultCh <- apiResult{ip, err}
		}(u)
	}

	// Close resultCh after all goroutines finish.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var lastErr error
	timeout := time.After(apiTotalTimeout)

	for {
		select {
		case result, ok := <-resultCh:
			if !ok {
				// All goroutines completed, no success.
				if !quiet {
					log.Error("All APIs failed. Tried %d URLs.", len(urls))
				}
				return nil, fmt.Errorf("all API requests failed: %v", lastErr)
			}

			if result.err == nil {
				// Success — signal other goroutines to stop.
				once.Do(func() { close(doneCh) })

				if !quiet {
					log.Info("API succeeded: %s", result.ip)
				}

				info := IPv6Info{
					IP:           net.ParseIP(result.ip),
					PreferredLft: infiniteLifetime,
					ValidLft:     infiniteLifetime,
				}
				PopulateInfo(&info)
				return []IPv6Info{info}, nil
			}

			// This API failed — log and wait for others.
			if !quiet {
				log.Error("API failed: %v", result.err)
			}
			lastErr = result.err

		case <-timeout:
			// Total timeout exceeded.
			once.Do(func() { close(doneCh) })
			if !quiet {
				log.Error("API query timeout (%v)", apiTotalTimeout)
			}
			return nil, errors.New("API query timeout")
		}
	}
}
