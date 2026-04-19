package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	xnet "golang.org/x/net/proxy"
)

// CloudflareProvider implements Cloudflare-specific logic
type CloudflareProvider struct {
	config   Config
	apiToken string
	proxyURL string
	client   *http.Client
}

// Config minimal config interface for Cloudflare provider
type Config interface {
	GetProxy() string
}

// SimpleConfig simple config implementation
type SimpleConfig struct {
	Proxy string
}

func (c *SimpleConfig) GetProxy() string {
	return c.Proxy
}

const (
	cloudflareAPI  = "https://api.cloudflare.com/client/v4"
	zonesEndpoint  = cloudflareAPI + "/zones"
	defaultRetries = 3
	baseDelay      = 1 * time.Second
)

// NewProvider constructor with apiToken
func NewProvider(cfg Config, apiToken string) *CloudflareProvider {
	p := &CloudflareProvider{
		config:   cfg,
		apiToken: apiToken,
	}
	// 保存 proxyURL 以便 GetProxy 返回
	if cfg != nil {
		p.proxyURL = cfg.GetProxy()
	}
	p.client = p.createHTTPClient()
	return p
}

// createHTTPClient creates (or recreates) an HTTP client with optional proxy settings.
func (p *CloudflareProvider) createHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	if p.config != nil && p.config.GetProxy() != "" {
		proxyURL, err := url.Parse(p.config.GetProxy())
		if err == nil {
			switch strings.ToLower(proxyURL.Scheme) {
			case "http", "https":
				transport.Proxy = http.ProxyURL(proxyURL)
			case "socks5", "socks5h":
				var auth *xnet.Auth
				if proxyURL.User != nil {
					pw, _ := proxyURL.User.Password()
					auth = &xnet.Auth{User: proxyURL.User.Username(), Password: pw}
				}
				dialer, err := xnet.SOCKS5("tcp", proxyURL.Host, auth, xnet.Direct)
				if err == nil {
					transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
						return dialer.Dial(network, addr)
					}
				}
			}
		}
	}

	return &http.Client{Timeout: 15 * time.Second, Transport: transport}
}

// Name returns the provider name
func (p *CloudflareProvider) Name() string {
	return "cloudflare"
}

// SetProxy sets the proxy URL for the provider
func (p *CloudflareProvider) SetProxy(proxyURL string) error {
	p.proxyURL = proxyURL
	// 同时更新 config 中的 proxy
	if p.config == nil {
		p.config = &SimpleConfig{Proxy: proxyURL}
	} else {
		// 如果 config 是 SimpleConfig 类型，更新它
		if sc, ok := p.config.(*SimpleConfig); ok {
			sc.Proxy = proxyURL
		}
	}
	// Recreate HTTP client to apply new proxy settings
	p.client = p.createHTTPClient()
	return nil
}

// GetProxy returns the current proxy URL
func (p *CloudflareProvider) GetProxy() string {
	return p.proxyURL
}

// cfRequest with retry
func (p *CloudflareProvider) cfRequest(ctx context.Context, method string, endpoint string, data interface{}) (*http.Response, error) {
	var jsonBody []byte
	if data != nil {
		var err error
		jsonBody, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request data: %w", err)
		}
	}

	// Ensure client is initialized (especially if proxy was updated via SetProxy)
	if p.client == nil {
		p.client = p.createHTTPClient()
	}

	for attempt := 0; attempt <= defaultRetries; attempt++ {
		var body io.Reader
		if jsonBody != nil {
			body = bytes.NewReader(jsonBody)
		}

		req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+p.apiToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.client.Do(req)
		if err != nil {
			if attempt == defaultRetries {
				return nil, fmt.Errorf("API request failed after %d retries: %w", defaultRetries, err)
			}
			delay := baseDelay * time.Duration(1<<attempt)
			time.Sleep(delay)
			continue
		}

		if resp.StatusCode >= 500 && attempt < defaultRetries {
			resp.Body.Close() // 关闭响应体后再重试
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("max retries exceeded")
}

// GetZoneID returns the Cloudflare Zone ID for the given zone name
func (p *CloudflareProvider) GetZoneID(ctx context.Context, zoneName string) (string, error) {
	reqURL := zonesEndpoint + "?name=" + zoneName
	resp, err := p.cfRequest(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode Zone ID response: %w", err)
	}

	if !result.Success || len(result.Result) == 0 {
		errMsg := "unknown error"
		if len(result.Errors) > 0 {
			errMsg = fmt.Sprintf("Code %d: %s", result.Errors[0].Code, result.Errors[0].Message)
		}
		return "", fmt.Errorf("failed to find zone %s. API error: %s", zoneName, errMsg)
	}

	return result.Result[0].ID, nil
}

// UpsertRecord creates or updates a DNS record
// zoneID should be passed in the extra map as "zoneID"
func (p *CloudflareProvider) UpsertRecord(ctx context.Context, zoneName, recordName, ip string, ttl int, extra map[string]interface{}) (bool, error) {
	fqdn := recordName + "." + zoneName
	recordType := "AAAA"

	// Get zoneID from extra params
	zoneID, ok := extra["zoneID"].(string)
	if !ok || zoneID == "" {
		return false, fmt.Errorf("zoneID is required in extra parameters")
	}

	// Get proxied from extra params
	proxied := false
	if v, ok := extra["proxied"]; ok {
		proxied = v.(bool)
	}

	searchURL := fmt.Sprintf("%s/%s/dns_records?type=%s&name=%s", zonesEndpoint, zoneID, recordType, fqdn)
	resp, err := p.cfRequest(ctx, "GET", searchURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to search existing DNS record: %w", err)
	}
	defer resp.Body.Close()

	var searchResult struct {
		Success bool `json:"success"`
		Result  []struct {
			ID      string `json:"id"`
			Content string `json:"content"`
			Proxied bool   `json:"proxied"`
			TTL     int    `json:"ttl"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return false, fmt.Errorf("failed to decode DNS search response: %w", err)
	}

	if !searchResult.Success {
		return false, fmt.Errorf("DNS search failed. API error: %s", searchResult.Errors[0].Message)
	}

	newRecordData := map[string]interface{}{
		"type":    recordType,
		"name":    fqdn,
		"content": ip,
		"ttl":     ttl,
		"proxied": proxied,
	}

	var method, apiEndpoint string

	if len(searchResult.Result) > 0 {
		existing := searchResult.Result[0]
		// Check if update is needed
		if existing.Content == ip && existing.Proxied == proxied && existing.TTL == ttl {
			return true, nil
		}
		recordID := existing.ID
		method = "PUT"
		apiEndpoint = fmt.Sprintf("%s/%s/dns_records/%s", zonesEndpoint, zoneID, recordID)
	} else {
		method = "POST"
		apiEndpoint = fmt.Sprintf("%s/%s/dns_records", zonesEndpoint, zoneID)
	}

	resp, err = p.cfRequest(ctx, method, apiEndpoint, newRecordData)
	if err != nil {
		return false, fmt.Errorf("API call failed during %s: %w", method, err)
	}
	defer resp.Body.Close()

	var updateResult struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&updateResult); err != nil {
		return false, fmt.Errorf("failed to decode API %s response: %w", method, err)
	}

	if !updateResult.Success {
		errMsg := updateResult.Errors[0].Message
		return false, fmt.Errorf("Cloudflare API %s failed (Code %d): %s", method, updateResult.Errors[0].Code, errMsg)
	}

	return true, nil
}

// UpsertDNSRecord is a wrapper for backward compatibility
func (p *CloudflareProvider) UpsertDNSRecord(zoneName, recordName, ip, zoneID string, ttl int, proxied bool) (bool, error) {
	ctx := context.Background()
	extra := map[string]interface{}{"proxied": proxied, "zoneID": zoneID}
	return p.UpsertRecord(ctx, zoneName, recordName, ip, ttl, extra)
}

// GetZoneID is a wrapper for backward compatibility
func (p *CloudflareProvider) GetZoneIDLegacy(zoneName string) (string, error) {
	ctx := context.Background()
	return p.GetZoneID(ctx, zoneName)
}
