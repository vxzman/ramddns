package aliyun

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	xnet "golang.org/x/net/proxy"
)

// AliyunProvider implements Aliyun DNS-specific logic
type AliyunProvider struct {
	accessKeyID     string
	accessKeySecret string
	proxyURL        string
	client          *http.Client
}

const (
	aliyunAPIEndpoint = "https://alidns.aliyuncs.com/"
	apiVersion        = "2015-01-09"
	defaultRetries    = 3
	baseDelay         = 1 * time.Second
)

// NewProvider constructor
func NewProvider(accessKeyID, accessKeySecret string) *AliyunProvider {
	p := &AliyunProvider{
		accessKeyID:     accessKeyID,
		accessKeySecret: accessKeySecret,
	}
	client, err := p.createHTTPClient()
	if err != nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	p.client = client
	return p
}

// Name returns the provider name
func (p *AliyunProvider) Name() string {
	return "aliyun"
}

// SetProxy makes later API calls use the given proxy URL.
func (p *AliyunProvider) SetProxy(proxyURL string) error {
	p.proxyURL = proxyURL
	client, err := p.createHTTPClient()
	if err != nil {
		return err
	}
	p.client = client
	return nil
}

// GetProxy returns the current proxy URL.
func (p *AliyunProvider) GetProxy() string {
	return p.proxyURL
}

func (p *AliyunProvider) createHTTPClient() (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if p.proxyURL != "" {
		proxyURL, err := url.Parse(p.proxyURL)
		if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
			return nil, fmt.Errorf("invalid proxy URL")
		}
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
			if err != nil {
				return nil, fmt.Errorf("socks proxy: %w", err)
			}
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
		}
	}
	return &http.Client{Timeout: 15 * time.Second, Transport: transport}, nil
}

// sign creates the HMAC-SHA1 signature
func (p *AliyunProvider) sign(signString string) string {
	h := hmac.New(sha1.New, []byte(p.accessKeySecret+"&"))
	h.Write([]byte(signString))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// signRequest signs and sends the request
func (p *AliyunProvider) signRequest(ctx context.Context, params map[string]string) (*http.Response, error) {
	params["AccessKeyId"] = p.accessKeyID
	params["Format"] = "JSON"
	params["SignatureMethod"] = "HMAC-SHA1"
	params["SignatureVersion"] = "1.0"
	params["SignatureNonce"] = fmt.Sprintf("%d", time.Now().UnixNano())
	params["Timestamp"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	params["Version"] = apiVersion

	// Generate signature
	signature := p.generateSignature(params)
	params["Signature"] = signature

	// Build query string
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	reqURL := aliyunAPIEndpoint + "?" + values.Encode()

	for attempt := 0; attempt <= defaultRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, err
		}

		client := p.client
		if client == nil {
			client = http.DefaultClient
		}
		resp, err := client.Do(req)
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

// generateSignature generates the signature string
func (p *AliyunProvider) generateSignature(params map[string]string) string {
	// Sort parameters by key
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build canonicalized query string
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(url.QueryEscape(k))
		sb.WriteByte('=')
		sb.WriteString(url.QueryEscape(params[k]))
	}

	// Build string to sign
	signString := "GET&" + url.QueryEscape("/") + "&" + url.QueryEscape(sb.String())

	return p.sign(signString)
}

// GetRecordID gets the DNS record ID for the given domain
func (p *AliyunProvider) GetRecordID(ctx context.Context, domain string) (string, error) {
	params := map[string]string{
		"Action":    "DescribeSubDomainRecords",
		"SubDomain": domain,
	}

	resp, err := p.signRequest(ctx, params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		TotalCount    int `json:"TotalCount"`
		DomainRecords struct {
			Record []struct {
				RecordID string `json:"RecordId"`
				RR       string `json:"RR"`
				Type     string `json:"Type"`
				Value    string `json:"Value"`
			} `json:"Record"`
		} `json:"DomainRecords"`
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != "" {
		return "", fmt.Errorf("API error: %s - %s", result.Code, result.Message)
	}

	if result.TotalCount == 0 || len(result.DomainRecords.Record) == 0 {
		return "", nil // Record not found
	}

	// Find AAAA record
	for _, record := range result.DomainRecords.Record {
		if record.Type == "AAAA" {
			return record.RecordID, nil
		}
	}

	// If no AAAA record, return the first record ID
	return result.DomainRecords.Record[0].RecordID, nil
}

// UpsertRecord creates or updates a DNS record (implements provider.Provider interface)
func (p *AliyunProvider) UpsertRecord(ctx context.Context, zone, record, ip string, ttl int, extra map[string]interface{}) (bool, error) {
	return p.UpsertDNSRecord(ctx, zone, record, ip, ttl)
}

// UpsertDNSRecord creates or updates a DNS record
func (p *AliyunProvider) UpsertDNSRecord(ctx context.Context, zone, record, ip string, ttl int) (bool, error) {
	// Build full domain name
	var fullDomain string
	if record == "@" {
		fullDomain = zone
	} else {
		fullDomain = record + "." + zone
	}

	// Get existing record ID
	recordID, err := p.GetRecordID(ctx, fullDomain)
	if err != nil {
		return false, fmt.Errorf("failed to get record ID: %w", err)
	}

	var params map[string]string

	if recordID == "" {
		// Create new record
		params = map[string]string{
			"Action":     "AddDomainRecord",
			"DomainName": zone,
			"RR":         record,
			"Type":       "AAAA",
			"Value":      ip,
		}
		if ttl > 0 {
			params["TTL"] = strconv.Itoa(ttl)
		}
	} else {
		// Update existing record
		params = map[string]string{
			"Action":   "UpdateDomainRecord",
			"RecordId": recordID,
			"RR":       record,
			"Type":     "AAAA",
			"Value":    ip,
		}
		if ttl > 0 {
			params["TTL"] = strconv.Itoa(ttl)
		}
	}

	resp, err := p.signRequest(ctx, params)
	if err != nil {
		return false, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		RecordID string `json:"RecordId"`
		Code     string `json:"Code"`
		Message  string `json:"Message"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != "" {
		return false, fmt.Errorf("API error: %s - %s", result.Code, result.Message)
	}

	return true, nil
}
