package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"ipflow/internal/log"
)

// IPSource source for obtaining IP
type IPSource struct {
	Interface string   `json:"interface,omitempty"`
	URLs      []string `json:"urls,omitempty"` // 支持多个 URL
}

// GeneralConfig global configuration settings
type GeneralConfig struct {
	GetIP     IPSource `json:"get_ip"`
	WorkDir   string   `json:"work_dir,omitempty"`
	LogOutput string   `json:"log_output,omitempty"`
	Proxy     string   `json:"proxy,omitempty"` // 全局代理配置
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
	Proxied    bool              `json:"proxied,omitempty"` // Cloudflare only
	UseProxy   bool              `json:"use_proxy,omitempty"`
	Cloudflare *CloudflareRecord `json:"cloudflare,omitempty"`
	Aliyun     *AliyunRecord     `json:"aliyun,omitempty"`
}

// Config main configuration structure
type Config struct {
	General GeneralConfig  `json:"general"`
	Records []RecordConfig `json:"records"`
}

// ReadConfig reads and validates config
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
		// 使用增强的 JSON 错误处理
		errorDetail := FormatJSONParseError(data, err)
		log.Error("配置文件 JSON 格式错误:\n%s", errorDetail)
		return nil, ""
	}

	// 验证配置（在扩展环境变量之前，检查是否使用明文密钥）
	if err := validateConfig(&config); err != nil {
		log.Error("Invalid config: %v", err)
		return nil, ""
	}

	// 扩展环境变量
	expandConfigEnvVars(&config)

	// 再次验证扩展后的配置（检查环境变量是否为空）
	if err := validateConfigExpanded(&config); err != nil {
		log.Error("Invalid config after environment variable expansion: %v", err)
		return nil, ""
	}

	return &config, configFile
}

// expandConfigEnvVars expands environment variables in sensitive config fields
func expandConfigEnvVars(cfg *Config) {
	// 扩展全局代理配置
	if cfg.General.Proxy != "" {
		cfg.General.Proxy = expandEnv(cfg.General.Proxy)
	}

	// 扩展每个记录的敏感信息
	for i := range cfg.Records {
		record := &cfg.Records[i]

		// Cloudflare 配置
		if record.Cloudflare != nil {
			if record.Cloudflare.APIToken != "" {
				record.Cloudflare.APIToken = expandEnv(record.Cloudflare.APIToken)
			}
			if record.Cloudflare.ZoneID != "" {
				record.Cloudflare.ZoneID = expandEnv(record.Cloudflare.ZoneID)
			}
		}

		// 阿里云配置
		if record.Aliyun != nil {
			if record.Aliyun.AccessKeyID != "" {
				record.Aliyun.AccessKeyID = expandEnv(record.Aliyun.AccessKeyID)
			}
			if record.Aliyun.AccessKeySecret != "" {
				record.Aliyun.AccessKeySecret = expandEnv(record.Aliyun.AccessKeySecret)
			}
		}
	}
}

// expandEnv expands environment variables with support for default values
// Supports: ${VAR}, ${VAR:-default}, ${VAR-default}
func expandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		// 处理默认值语法 ${VAR:-default} 或 ${VAR-default}
		var defaultValue string
		var hasDefault bool

		if idx := strings.Index(key, ":-"); idx != -1 {
			// ${VAR:-default} - 如果 VAR 未设置或为空，使用 default
			defaultValue = key[idx+2:]
			key = key[:idx]
			hasDefault = true
		} else if idx := strings.Index(key, "-"); idx != -1 {
			// ${VAR-default} - 如果 VAR 未设置，使用 default
			defaultValue = key[idx+1:]
			key = key[:idx]
			hasDefault = true
		}

		val := os.Getenv(key)
		if val == "" && hasDefault {
			return defaultValue
		}
		return val
	})
}

// isEnvVarReference checks if a string looks like an environment variable reference ${...}
func isEnvVarReference(s string) bool {
	if len(s) < 4 { // Minimum: ${X}
		return false
	}
	if !strings.HasPrefix(s, "${") || !strings.HasSuffix(s, "}") {
		return false
	}
	return true
}

// validateNoPlaintextSecret validates that sensitive values are not stored in plaintext
func validateNoPlaintextSecret(value, fieldName string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", fieldName)
	}
	if !isEnvVarReference(value) {
		return fmt.Errorf("%s must use environment variable reference (e.g., ${VAR_NAME}), plaintext secrets are not allowed for security reasons", fieldName)
	}
	return nil
}

// validateConfig validates the configuration (before env expansion)
// This checks for plaintext secrets and basic configuration validity
func validateConfig(cfg *Config) error {
	if len(cfg.Records) == 0 {
		return fmt.Errorf("at least one record must be configured")
	}

	// 检查 IP 源配置
	hasInterface := cfg.General.GetIP.Interface != ""
	hasURL := len(cfg.General.GetIP.URLs) > 0
	if !hasInterface && !hasURL {
		return fmt.Errorf("either 'get_ip.interface' or 'get_ip.urls' must be configured")
	}

	// 验证全局代理配置
	if cfg.General.Proxy != "" {
		if err := validateProxyURL(cfg.General.Proxy); err != nil {
			return fmt.Errorf("invalid global proxy: %w", err)
		}
	}

	// 验证每个记录
	for i, record := range cfg.Records {
		if record.Provider == "" {
			return fmt.Errorf("record[%d]: provider is required", i)
		}
		if record.Zone == "" {
			return fmt.Errorf("record[%d]: zone is required", i)
		}
		if record.Record == "" {
			return fmt.Errorf("record[%d]: record name is required", i)
		}

		// 验证记录级代理配置
		if record.UseProxy && cfg.General.Proxy == "" {
			return fmt.Errorf("record[%d]: use_proxy is true but no global proxy configured", i)
		}

		// 验证凭证（检查明文密钥）
		switch record.Provider {
		case "cloudflare":
			if record.Cloudflare == nil {
				return fmt.Errorf("record[%d]: cloudflare configuration is missing", i)
			}
			// Security check: api_token must use environment variable reference
			if err := validateNoPlaintextSecret(record.Cloudflare.APIToken, fmt.Sprintf("record[%d].cloudflare.api_token", i)); err != nil {
				return err
			}
		case "aliyun":
			if record.Aliyun == nil {
				return fmt.Errorf("record[%d]: aliyun configuration is missing", i)
			}
			// Security check: access_key_id must use environment variable reference
			if err := validateNoPlaintextSecret(record.Aliyun.AccessKeyID, fmt.Sprintf("record[%d].aliyun.access_key_id", i)); err != nil {
				return err
			}
			// Security check: access_key_secret must use environment variable reference
			if err := validateNoPlaintextSecret(record.Aliyun.AccessKeySecret, fmt.Sprintf("record[%d].aliyun.access_key_secret", i)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("record[%d]: unsupported provider '%s'", i, record.Provider)
		}
	}

	return nil
}

// validateConfigExpanded validates the configuration after env expansion
// This checks that environment variables were set correctly
func validateConfigExpanded(cfg *Config) error {
	for i, record := range cfg.Records {
		switch record.Provider {
		case "cloudflare":
			if record.Cloudflare.APIToken == "" {
				return fmt.Errorf("record[%d]: cloudflare.api_token environment variable is not set or empty", i)
			}
		case "aliyun":
			if record.Aliyun.AccessKeyID == "" {
				return fmt.Errorf("record[%d]: aliyun.access_key_id environment variable is not set or empty", i)
			}
			if record.Aliyun.AccessKeySecret == "" {
				return fmt.Errorf("record[%d]: aliyun.access_key_secret environment variable is not set or empty", i)
			}
		}
	}
	return nil
}

// validateProxyURL validates proxy URL format
func validateProxyURL(proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("proxy must include scheme (e.g., 'socks5://', 'http://')")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "socks5" && scheme != "socks5h" {
		return fmt.Errorf("unsupported proxy scheme '%s'", scheme)
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

// GetCacheFilePath returns the path for storing last ip
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

// WriteLastIP writes the ip to cache file
func WriteLastIP(path string, ip string) error {
	return os.WriteFile(path, []byte(ip), 0600)
}

// UpdateZoneIDCache saves Cloudflare Zone IDs to a local cache file.
// This avoids writing the full configuration (which may include secrets).
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

// ReadZoneIDCache reads a Zone ID cache file and returns a map of zone->zoneID.
// Returns nil map if file doesn't exist or cannot be parsed.
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
// Returns empty string if proxy should not be used
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
	// Default TTL
	if record.Provider == "cloudflare" {
		return 180
	}
	return 600
}
