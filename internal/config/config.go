package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ramddns/internal/log"
)

// IPSource specifies how to obtain the current IPv6 address.
type IPSource struct {
	Interface    string
	FallbackURLs []string
}

// RecordConfig is one DNS record after the config file has been flattened.
// Name is the RR label; "@" is the zone apex. Type is always AAAA.
type RecordConfig struct {
	Provider string
	Zone     string
	Name     string
	TTL      int
	Proxied  bool
	UseProxy bool

	APIToken string
	ZoneID   string

	AccessKeyID     string
	AccessKeySecret string
}

// Config is the runtime configuration.
type Config struct {
	IPSource IPSource
	Proxy    string
	Records  []RecordConfig
}

type fileConfig struct {
	Interface    string          `json:"interface"`
	FallbackURLs []string        `json:"fallback_urls"`
	Proxy        string          `json:"proxy"`
	Cloudflare   *fileCloudflare `json:"cloudflare"`
	Aliyun       *fileAliyun     `json:"aliyun"`
}

type fileCloudflare struct {
	Token   string         `json:"token"`
	Records []fileCFRecord `json:"records"`
}

type fileCFRecord struct {
	Name     string `json:"name"`
	ZoneID   string `json:"zone_id"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	UseProxy bool   `json:"use_proxy"`
}

type fileAliyun struct {
	AccessKeyID     string          `json:"access_key_id"`
	AccessKeySecret string          `json:"access_key_secret"`
	Records         []fileAliRecord `json:"records"`
}

type fileAliRecord struct {
	Name     string `json:"name"`
	TTL      int    `json:"ttl"`
	UseProxy bool   `json:"use_proxy"`
}

// ReadConfig reads and validates config from a JSON file.
// quiet is accepted for call-site compatibility and is unused.
func ReadConfig(path string, quiet bool) (*Config, string) {
	_ = quiet

	configFile, err := filepath.Abs(path)
	if err != nil {
		log.Error("Failed to resolve config path: %v", err)
		return nil, ""
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Error("Failed to read config file: %v", err)
		return nil, ""
	}

	cfg, err := parseConfig(data)
	if err != nil {
		log.Error("Invalid config: %v", err)
		return nil, ""
	}
	return cfg, configFile
}

func parseConfig(data []byte) (*Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var file fileConfig
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("配置文件 JSON 格式错误：%w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("配置文件 JSON 格式错误：extra data after JSON value")
	}

	cfg := &Config{
		Proxy: strings.TrimSpace(file.Proxy),
	}
	cfg.IPSource.Interface = strings.TrimSpace(file.Interface)
	for _, rawURL := range file.FallbackURLs {
		url := strings.TrimSpace(rawURL)
		if url == "" {
			return nil, fmt.Errorf("fallback_urls entries must be non-empty strings")
		}
		cfg.IPSource.FallbackURLs = append(cfg.IPSource.FallbackURLs, url)
	}

	if file.Cloudflare != nil {
		if err := appendCloudflare(cfg, file.Cloudflare); err != nil {
			return nil, err
		}
	}
	if file.Aliyun != nil {
		if err := appendAliyun(cfg, file.Aliyun); err != nil {
			return nil, err
		}
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func appendCloudflare(cfg *Config, cf *fileCloudflare) error {
	token := strings.TrimSpace(cf.Token)
	if token == "" {
		return fmt.Errorf("cloudflare.token is required")
	}
	if len(cf.Records) == 0 {
		return fmt.Errorf("cloudflare.records must contain at least one record")
	}
	for i, rec := range cf.Records {
		where := fmt.Sprintf("cloudflare.records[%d]", i)
		rr, zone, err := splitFQDN(rec.Name)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if rec.TTL < 0 {
			return fmt.Errorf("%s: ttl must not be negative", where)
		}
		cfg.Records = append(cfg.Records, RecordConfig{
			Provider: "cloudflare",
			Zone:     zone,
			Name:     rr,
			TTL:      rec.TTL,
			Proxied:  rec.Proxied,
			UseProxy: rec.UseProxy,
			APIToken: token,
			ZoneID:   strings.TrimSpace(rec.ZoneID),
		})
	}
	return nil
}

func appendAliyun(cfg *Config, ali *fileAliyun) error {
	keyID := strings.TrimSpace(ali.AccessKeyID)
	keySecret := strings.TrimSpace(ali.AccessKeySecret)
	if keyID == "" {
		return fmt.Errorf("aliyun.access_key_id is required")
	}
	if keySecret == "" {
		return fmt.Errorf("aliyun.access_key_secret is required")
	}
	if len(ali.Records) == 0 {
		return fmt.Errorf("aliyun.records must contain at least one record")
	}
	for i, rec := range ali.Records {
		where := fmt.Sprintf("aliyun.records[%d]", i)
		rr, zone, err := splitFQDN(rec.Name)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if rec.TTL < 0 {
			return fmt.Errorf("%s: ttl must not be negative", where)
		}
		cfg.Records = append(cfg.Records, RecordConfig{
			Provider:        "aliyun",
			Zone:            zone,
			Name:            rr,
			TTL:             rec.TTL,
			UseProxy:        rec.UseProxy,
			AccessKeyID:     keyID,
			AccessKeySecret: keySecret,
		})
	}
	return nil
}

// splitFQDN turns a DNS name into an RR label and a zone.
// Two labels are the zone apex. Three or more labels use the leftmost label
// as the RR. A multi-label public suffix such as example.com.cn cannot be
// distinguished from a host under that suffix.
func splitFQDN(raw string) (string, string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", "", fmt.Errorf("name is empty")
	}
	if strings.ContainsAny(s, " \t") {
		return "", "", fmt.Errorf("name must not contain whitespace")
	}
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "", "", fmt.Errorf("name is empty")
	}

	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return "", "", fmt.Errorf("name must contain at least two labels, for example example.com")
	}
	for _, label := range labels {
		if label == "" {
			return "", "", fmt.Errorf("name has an empty label")
		}
	}
	if len(labels) == 2 {
		return "@", s, nil
	}
	return labels[0], strings.Join(labels[1:], "."), nil
}

// RecordFQDN is the name shown to the user. Apex records are the zone itself.
func RecordFQDN(record *RecordConfig) string {
	if record.Name == "@" {
		return record.Zone
	}
	return record.Name + "." + record.Zone
}

// GetCacheFilePath returns the path for storing the last IP.
func GetCacheFilePath(configFile string, workDir string) string {
	if workDir != "" {
		if err := os.MkdirAll(workDir, 0755); err != nil {
			log.Error("Warning: Failed to create work_dir '%s'. Falling back to config file directory. Error: %v", workDir, err)
			return filepath.Join(filepath.Dir(configFile), "cache.lastip")
		}
		return filepath.Join(workDir, "cache.lastip")
	}
	return filepath.Join(filepath.Dir(configFile), "cache.lastip")
}

// GetRecordProxy returns the proxy URL for a specific record.
func GetRecordProxy(cfg *Config, record *RecordConfig) string {
	if !record.UseProxy {
		return ""
	}
	return cfg.Proxy
}

// GetRecordTTL returns the effective TTL for a record.
func GetRecordTTL(record *RecordConfig) int {
	if record.TTL > 0 {
		return record.TTL
	}
	if record.Provider == "cloudflare" {
		return 180
	}
	return 600
}

// CacheFileData holds the last IP read from the cache file.
type CacheFileData struct {
	LastIP string
}

// ParseCacheFile reads the cache file. The last non-empty line wins.
// A line with a space is an old "RFC3339 ip" history entry; the IP is the second field.
// A line without a space is the IP itself.
func ParseCacheFile(path string) CacheFileData {
	var data CacheFileData

	content, err := os.ReadFile(path)
	if err != nil {
		return data
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ip := line
		if space := strings.IndexByte(line, ' '); space >= 0 {
			ip = strings.TrimSpace(line[space+1:])
		}
		if ip != "" {
			data.LastIP = ip
		}
	}
	return data
}

// ReadLastIP reads the last IP from the cache file.
func ReadLastIP(path string) string {
	return ParseCacheFile(path).LastIP
}

// WriteLastIP overwrites the cache with a single IP line.
func WriteLastIP(path string, ip string) error {
	return os.WriteFile(path, []byte(ip+"\n"), 0600)
}

// UpdateZoneIDCache saves a Cloudflare Zone ID to a local cache file.
func UpdateZoneIDCache(path string, zone string, zoneID string) error {
	zoneIDs := make(map[string]string)
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &zoneIDs)
	}
	zoneIDs[zone] = zoneID

	out, err := json.MarshalIndent(zoneIDs, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0600)
}

// ReadZoneIDCache reads a Zone ID cache file.
func ReadZoneIDCache(path string) map[string]string {
	zoneIDs := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(data, &zoneIDs); err != nil {
		return nil
	}
	return zoneIDs
}
