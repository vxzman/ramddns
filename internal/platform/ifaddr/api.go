package ifaddr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"aiolos/internal/log"
)

// GetIPv6FromAPIs queries remote HTTP APIs for IPv6 addresses.
// Always uses direct connection (no proxy).
func GetIPv6FromAPIs(urls []string, quiet bool) ([]IPv6Info, error) {
	if len(urls) == 0 {
		return nil, errors.New("no IP API URL configured")
	}

	const retries = 2

	resultChan := make(chan struct {
		info []IPv6Info
		err  error
		url  string
	}, len(urls))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for _, u := range urls {
		go func(u string) {
			defer func() {
				if r := recover(); r != nil {
					resultChan <- struct {
						info []IPv6Info
						err  error
						url  string
					}{nil, fmt.Errorf("panic in fallback goroutine: %v", r), u}
				}
			}()

			client := &http.Client{
				Timeout: 15 * time.Second,
			}

			for attempt := 0; attempt <= retries; attempt++ {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if !quiet {
					log.Info("Querying API %s (attempt %d/%d)", u, attempt+1, retries+1)
				}

				resp, err := client.Get(u)
				if err != nil {
					if attempt == retries {
						resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("API request failed: %v", err), u}
					}
					if attempt < retries {
						time.Sleep(time.Second * 2)
					}
					continue
				}

				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					if attempt == retries {
						resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("failed to read response body: %v", err), u}
					}
					continue
				}

				if resp.StatusCode != http.StatusOK {
					if attempt == retries {
						resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("API returned HTTP %d", resp.StatusCode), u}
					}
					continue
				}

				responseBody := strings.TrimSpace(string(body))
				if responseBody == "" {
					if attempt == retries {
						resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("empty response from API"), u}
					}
					continue
				}

				ipStr := strings.Split(responseBody, "\n")[0]
				ipStr = strings.TrimSpace(ipStr)

				if ipStr == "" {
					if attempt == retries {
						resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("no valid IP found in response: %s", responseBody), u}
					}
					continue
				}

				ip := net.ParseIP(ipStr)
				if ip == nil {
					if attempt == retries {
						resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("invalid IP format: %s", ipStr), u}
					}
					continue
				}

				if ip.To4() != nil {
					if attempt == retries {
						resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("returned IPv4 instead of IPv6: %s", ipStr), u}
					}
					continue
				}

				if ip.IsLinkLocalUnicast() || ip.IsLoopback() || IsPrivateOrLocalIP(ip) {
					if attempt == retries {
						resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("invalid address type: %s", ipStr), u}
					}
					continue
				}

				info := IPv6Info{
					IP:           ip,
					PreferredLft: time.Hour * 24 * 365 * 10,
					ValidLft:     time.Hour * 24 * 365 * 10,
				}
				PopulateInfo(&info)

				if !quiet {
					log.Info("API %s succeeded: %s", u, ipStr)
				}
				resultChan <- struct {
					info []IPv6Info
					err  error
					url  string
				}{[]IPv6Info{info}, nil, u}
				return
			}
		}(u)
	}

	var lastErr error
	for range urls {
		select {
		case res := <-resultChan:
			if res.err == nil {
				return res.info, nil
			}
			lastErr = res.err
			if !quiet {
				log.Error("API %s failed: %v", res.url, res.err)
			}
		case <-ctx.Done():
			return nil, errors.New("all API requests timed out")
		}
	}

	if !quiet {
		log.Error("All APIs failed. Tried %d URLs: %v", len(urls), urls)
	}
	return nil, lastErr
}
