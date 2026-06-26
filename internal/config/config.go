package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ramddns/internal/log"
)

// IPSource specifies how to obtain the current IP address.
type IPSource struct {
	Interface    string   `json:"interface,omitempty"`
	FallbackURLs []string `json:"fallback_urls,omitempty"`
}

// RecordConfig is a single DNS record configuration.
// Provider-specific fields (api_token, zone_id, access_key_id,
// access_key_secret) are flat at the record level — the provider
// ignores the ones it doesn't need.
type RecordConfig struct {
	Provider string `json:"provider"`
	Zone     string `json:"zone"`
	Name     string `json:"name"` // subdomain, "@" for root
	Type     string `json:"type,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
	Proxied  bool   `json:"proxied,omitempty"`  // Cloudflare CDN proxy
	UseProxy bool   `json:"use_proxy,omitempty"` // use global proxy for this record

	// Cloudflare
	APIToken string `json:"api_token,omitempty"`
	ZoneID   string `json:"zone_id,omitempty"`

	// Aliyun
	AccessKeyID     string `json:"access_key_id,omitempty"`
	AccessKeySecret string `json:"access_key_secret,omitempty"`
}

// Config is the root configuration structure.
type Config struct {
	Env      map[string]string `json:"env,omitempty"`
	IPSource IPSource          `json:"ip_source"`
	Proxy    string            `json:"proxy,omitempty"`
	Records  []RecordConfig    `json:"records"`
}

// ReadConfig reads and validates config from a JSON file.
func ReadConfig(path string, quiet bool) (*Config, string) {
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

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.Error("配置文件 JSON 格式错误：%v", err)
		return nil, ""
	}

	if err := validateConfig(&config); err != nil {
		log.Error("Invalid config: %v", err)
		return nil, ""
	}

	return &config, configFile
}

// resolveValue resolves $name from cfg.Env.
// Supports $name syntax only (e.g., $cf_token), not ${name}.
func resolveValue(s string, cfg *Config) string {
	if !strings.HasPrefix(s, "$") || strings.HasPrefix(s, "${") {
		return s
	}
	name := s[1:]
	if cfg.Env == nil {
		return ""
	}
	return cfg.Env[name]
}

// ResolveSecrets resolves $name references in record fields against cfg.Env.
func ResolveSecrets(cfg *Config) error {
	resolve := func(val string) (string, error) {
		if val == "" {
			return val, nil
		}
		if strings.HasPrefix(val, "$") && !strings.HasPrefix(val, "${") {
			name := val[1:]
			if cfg.Env == nil {
				return "", fmt.Errorf("no env section in config to resolve %s", name)
			}
			envVal, ok := cfg.Env[name]
			if !ok || envVal == "" {
				return "", fmt.Errorf("env variable %s is not set", name)
			}
			return envVal, nil
		}
		return val, nil
	}

	// Resolve proxy
	if cfg.Proxy != "" {
		r, err := resolve(cfg.Proxy)
		if err != nil {
			return err
		}
		cfg.Proxy = r
	}

	// Resolve record secrets
	for i := range cfg.Records {
		rec := &cfg.Records[i]

		if rec.APIToken != "" {
			r, err := resolve(rec.APIToken)
			if err != nil {
				return err
			}
			rec.APIToken = r
		}
		if rec.ZoneID != "" {
			r, err := resolve(rec.ZoneID)
			if err != nil {
				return err
			}
			rec.ZoneID = r
		}
		if rec.AccessKeyID != "" {
			r, err := resolve(rec.AccessKeyID)
			if err != nil {
				return err
			}
			rec.AccessKeyID = r
		}
		if rec.AccessKeySecret != "" {
			r, err := resolve(rec.AccessKeySecret)
			if err != nil {
				return err
			}
			rec.AccessKeySecret = r
		}
	}

	return nil
}

// WriteConfig writes config to the given path.
func WriteConfig(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// GetCacheFilePath returns the path for storing last IP and history.
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

// GetRecordType returns the record type, defaulting to "AAAA".
func GetRecordType(record *RecordConfig) string {
	if record.Type != "" {
		return record.Type
	}
	return "AAAA"
}

// --- Cache file helpers (unchanged logic) ---

// IPHistoryEntry represents a single IP change record.
type IPHistoryEntry struct {
	Timestamp time.Time
	IP        string
}

// CacheFileData holds parsed cache file contents.
type CacheFileData struct {
	LastIP  string
	History []IPHistoryEntry
}

// ParseCacheFile reads and parses the cache file.
// Format (one entry per line): <ISO8601_timestamp> <ip>
func ParseCacheFile(path string) CacheFileData {
	data := CacheFileData{History: make([]IPHistoryEntry, 0)}

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

		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			ts, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
			if err == nil {
				ip := strings.TrimSpace(parts[1])
				if ip != "" {
					data.History = append(data.History, IPHistoryEntry{
						Timestamp: ts,
						IP:        ip,
					})
				}
			}
		}
	}

	if len(data.History) > 0 {
		data.LastIP = data.History[len(data.History)-1].IP
	}

	return data
}

// WriteCacheFile writes the cache data to file.
func WriteCacheFile(path string, data CacheFileData) error {
	var sb strings.Builder

	for _, entry := range data.History {
		sb.WriteString(fmt.Sprintf("%s %s\n", entry.Timestamp.Format(time.RFC3339), entry.IP))
	}

	return os.WriteFile(path, []byte(sb.String()), 0600)
}

// AppendIPHistory records an IP change and writes the updated cache file.
func AppendIPHistory(path string, newIP string) (string, error) {
	data := ParseCacheFile(path)
	oldIP := data.LastIP

	data.LastIP = newIP
	data.History = append(data.History, IPHistoryEntry{
		Timestamp: time.Now().UTC(),
		IP:        newIP,
	})

	if err := WriteCacheFile(path, data); err != nil {
		return oldIP, err
	}

	return oldIP, nil
}

// ReadLastIP reads the last IP from cache file (deprecated, use ParseCacheFile).
func ReadLastIP(path string) string {
	return ParseCacheFile(path).LastIP
}

// WriteLastIP writes the IP to cache file (deprecated, use WriteCacheFile).
func WriteLastIP(path string, ip string) error {
	data := ParseCacheFile(path)
	data.LastIP = ip
	data.History = append(data.History, IPHistoryEntry{
		Timestamp: time.Now().UTC(),
		IP:        ip,
	})
	return WriteCacheFile(path, data)
}

// --- Zone ID cache ---

// UpdateZoneIDCache saves Cloudflare Zone IDs to a local cache file.
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
