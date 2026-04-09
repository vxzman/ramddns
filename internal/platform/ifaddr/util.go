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

	"ipflow/internal/log"
)

// GetIPv6FromAPIs queries remote APIs for IPv6 addresses
// 始终直连，不使用代理
func GetIPv6FromAPIs(urls []string, quiet bool) ([]IPv6Info, error) {
	if len(urls) == 0 {
		return nil, errors.New("no IP API URL configured")
	}

	const retries = 2

	// create result channel for concurrent requests
	resultChan := make(chan struct {
		info []IPv6Info
		err  error
		url  string
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for _, u := range urls {
		go func(u string) {
			defer func() {
				if r := recover(); r != nil {
					select {
					case resultChan <- struct {
						info []IPv6Info
						err  error
						url  string
					}{nil, fmt.Errorf("panic in fallback goroutine: %v", r), u}:
					case <-ctx.Done():
						return
					}
				}
			}()

			// Always use direct connection (no proxy) for IP retrieval
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
						select {
						case resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("API request failed: %v", err), u}:
						case <-ctx.Done():
							return
						}
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
						select {
						case resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("failed to read response body: %v", err), u}:
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				if resp.StatusCode != http.StatusOK {
					if attempt == retries {
						select {
						case resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("API returned HTTP %d", resp.StatusCode), u}:
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				// Parse IP from response
				responseBody := strings.TrimSpace(string(body))
				if responseBody == "" {
					if attempt == retries {
						select {
						case resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("empty response from API"), u}:
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				ipStr := strings.Split(responseBody, "\n")[0]
				ipStr = strings.TrimSpace(ipStr)

				if ipStr == "" {
					if attempt == retries {
						select {
						case resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("no valid IP found in response: %s", responseBody), u}:
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				ip := net.ParseIP(ipStr)
				if ip == nil {
					if attempt == retries {
						select {
						case resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("invalid IP format: %s", ipStr), u}:
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				// Verify it's an IPv6 address
				if ip.To4() != nil {
					if attempt == retries {
						select {
						case resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("returned IPv4 instead of IPv6: %s", ipStr), u}:
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				// Check address type
				if ip.IsLinkLocalUnicast() || ip.IsLoopback() || IsPrivateOrLocalIP(ip) {
					if attempt == retries {
						select {
						case resultChan <- struct {
							info []IPv6Info
							err  error
							url  string
						}{nil, fmt.Errorf("invalid address type: %s", ipStr), u}:
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				// Successfully parsed IPv6 address
				// API 返回的 IP 视为永久有效（静态 IP）
				info := IPv6Info{
					IP:           ip,
					PreferredLft: time.Hour * 24 * 365 * 10,
					ValidLft:     time.Hour * 24 * 365 * 10,
				}
				PopulateInfo(&info)

				if !quiet {
					log.Info("API %s succeeded: %s", u, ipStr)
				}
				select {
				case resultChan <- struct {
					info []IPv6Info
					err  error
					url  string
				}{[]IPv6Info{info}, nil, u}:
				case <-ctx.Done():
					return
				}
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
