package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aiolos/internal/log"
)

// IPSource source for obtaining IP
type IPSource struct {
	Interface string   `json:"interface,omitempty"`
	URLs      []string `json:"urls,omitempty"`
}

// GeneralConfig global configuration settings
type GeneralConfig struct {
	GetIP     IPSource `json:"get_ip"`
	WorkDir   string   `json:"work_dir,omitempty"`
	Proxy     string   `json:"proxy,omitempty"`
}

// CloudflareRecord Cloudflare provider specific settings
type CloudflareRecord struct {
	APIToken string `json:"api_token"`
	ZoneID   string `json:"zone_id,omitempty"`
	Proxied  bool   `json:"proxied,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
}

// AliyunRecord Aliyun provider specific settings
type AliyunRecord struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	TTL             int    `json:"ttl,omitempty"`
}

// RecordConfig single DNS record configuration
type RecordConfig struct {
	Provider   string            `json:"provider"`
	Zone       string            `json:"zone"`
	Record     string            `json:"record"`
	TTL        int               `json:"ttl,omitempty"`
	Proxied    bool              `json:"proxied,omitempty"`
	UseProxy   bool              `json:"use_proxy,omitempty"`
	Cloudflare *CloudflareRecord `json:"cloudflare,omitempty"`
	Aliyun     *AliyunRecord     `json:"aliyun,omitempty"`
}

// Config main configuration structure
type Config struct {
	General     GeneralConfig     `json:"general"`
	Environment map[string]string `json:"environment,omitempty"`
	Records     []RecordConfig    `json:"records"`
}

// ReadConfig reads and validates config from JSON file
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

// resolveValue resolves $name from cfg.Environment
// Only supports $name syntax (e.g., $cloudflare_var)
func resolveValue(s string, cfg *Config) string {
	if !strings.HasPrefix(s, "$") || strings.HasPrefix(s, "${") {
		return s
	}
	name := s[1:]
	if cfg.Environment == nil {
		return ""
	}
	return cfg.Environment[name]
}

// ResolveSecrets resolves $name references from environment section.
// Simple plaintext resolution - no encryption support.
func ResolveSecrets(cfg *Config) error {
	// Helper to resolve a single value
	resolve := func(val string) (string, error) {
		if val == "" {
			return val, nil
		}
		if strings.HasPrefix(val, "$") && !strings.HasPrefix(val, "${") {
			name := val[1:]
			if cfg.Environment == nil {
				return "", fmt.Errorf("no environment section in config to resolve %s", name)
			}
			envVal, ok := cfg.Environment[name]
			if !ok || envVal == "" {
				return "", fmt.Errorf("environment variable %s is not set", name)
			}
			return envVal, nil
		}
		return val, nil
	}

	// Resolve general proxy
	if cfg.General.Proxy != "" {
		r, err := resolve(cfg.General.Proxy)
		if err != nil {
			return err
		}
		cfg.General.Proxy = r
	}

	// Resolve record secrets
	for i := range cfg.Records {
		rec := &cfg.Records[i]
		if rec.Cloudflare != nil {
			if rec.Cloudflare.APIToken != "" {
				r, err := resolve(rec.Cloudflare.APIToken)
				if err != nil {
					return err
				}
				rec.Cloudflare.APIToken = r
			}
			if rec.Cloudflare.ZoneID != "" {
				r, err := resolve(rec.Cloudflare.ZoneID)
				if err != nil {
					return err
				}
				rec.Cloudflare.ZoneID = r
			}
		}
		if rec.Aliyun != nil {
			if rec.Aliyun.AccessKeyID != "" {
				r, err := resolve(rec.Aliyun.AccessKeyID)
				if err != nil {
					return err
				}
				rec.Aliyun.AccessKeyID = r
			}
			if rec.Aliyun.AccessKeySecret != "" {
				r, err := resolve(rec.Aliyun.AccessKeySecret)
				if err != nil {
					return err
				}
				rec.Aliyun.AccessKeySecret = r
			}
		}
	}

	return nil
}

// WriteConfig writes config to the given path
func WriteConfig(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// GetCacheFilePath returns the path for storing last IP
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

// ReadLastIP reads the last IP from cache file
func ReadLastIP(path string) string {
	ip, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(ip))
}

// WriteLastIP writes the IP to cache file
func WriteLastIP(path string, ip string) error {
	return os.WriteFile(path, []byte(ip), 0600)
}

// UpdateZoneIDCache saves Cloudflare Zone IDs to a local cache file
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

// ReadZoneIDCache reads a Zone ID cache file
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

// GetRecordProxy returns the proxy URL for a specific record
func GetRecordProxy(cfg *Config, record *RecordConfig) string {
	if !record.UseProxy {
		return ""
	}
	return cfg.General.Proxy
}

// GetRecordTTL returns the effective TTL for a record
func GetRecordTTL(record *RecordConfig) int {
	if record.TTL > 0 {
		return record.TTL
	}
	if record.Provider == "cloudflare" {
		return 180
	}
	return 600
}
