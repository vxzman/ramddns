package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
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

// Vars is a map of user-defined variables that can be referenced in the config
// Usage in config values: ${var.NAME}
type Vars map[string]string

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
	General GeneralConfig     `json:"general"`
	Vars    map[string]string `json:"vars,omitempty"`
	Records []RecordConfig    `json:"records"`
}

// ReadConfig reads and validates config (parses JSON and performs structural validation).
// Secret resolution (env/vars/decryption) is performed separately by ResolveSecrets
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

	// 仅做结构和必需字段校验（不对密钥格式做强制限制）
	if err := validateConfig(&config); err != nil {
		log.Error("Invalid config: %v", err)
		return nil, ""
	}

	return &config, configFile
}

// resolveValueWithVars expands ${var.NAME} from cfg.Vars and environment variables
func resolveValueWithVars(s string, cfg *Config) string {
	// replace ${var.NAME}
	varRe := func(s string) string {
		// find all ${var.NAME}
		res := s
		for {
			start := strings.Index(res, "${var.")
			if start == -1 {
				break
			}
			end := strings.Index(res[start:], "}")
			if end == -1 {
				break
			}
			end += start
			name := res[start+6 : end]
			val := ""
			if cfg != nil && cfg.Vars != nil {
				val = cfg.Vars[name]
			}
			res = res[:start] + val + res[end+1:]
		}
		return res
	}

	s = varRe(s)
	// expand environment variables like ${ENV}
	return expandEnv(s)
}

// ResolveSecrets resolves env references, ${var.*} references and decrypts enc: values using a key file.
// key is read from baseDir/.aiolos.key (if exists). If enc: values exist but key missing, an error is returned.
func ResolveSecrets(cfg *Config, baseDir string) error {
	keyPath := filepath.Join(baseDir, ".aiolos.key")
	var key []byte
	if _, err := os.Stat(keyPath); err == nil {
		k, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("failed to read key file: %w", err)
		}
		key = k
	}

	// Resolve vars first
	for k, v := range cfg.Vars {
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "enc:") {
			if key == nil {
				return fmt.Errorf("encrypted var %s found but key file missing: %s", k, keyPath)
			}
			dec, err := decryptValue(v, key)
			if err != nil {
				return fmt.Errorf("failed to decrypt var %s: %w", k, err)
			}
			cfg.Vars[k] = dec
			continue
		}
		// expand env and other var refs inside var value
		cfg.Vars[k] = expandEnv(v)
	}

	// Helper to resolve a single value
	resolve := func(val string) (string, error) {
		if val == "" {
			return val, nil
		}
		// first replace ${var.NAME} and envs
		val = resolveValueWithVars(val, cfg)
		if strings.HasPrefix(val, "enc:") {
			if key == nil {
				return "", fmt.Errorf("encrypted value found but key file missing: %s", keyPath)
			}
			return decryptValue(val, key)
		}
		return val, nil
	}

	// General proxy
	if cfg.General.Proxy != "" {
		r, err := resolve(cfg.General.Proxy)
		if err != nil {
			return err
		}
		cfg.General.Proxy = r
	}

	// Records
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

// validateConfig validates the configuration (before env expansion)
// This checks basic configuration validity but does NOT enforce secrets to be env references.
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

		// 验证提供商结构是否存在
		switch record.Provider {
		case "cloudflare":
			if record.Cloudflare == nil {
				return fmt.Errorf("record[%d]: cloudflare configuration is missing", i)
			}
			if record.Cloudflare.APIToken == "" {
				return fmt.Errorf("record[%d].cloudflare.api_token cannot be empty", i)
			}
		case "aliyun":
			if record.Aliyun == nil {
				return fmt.Errorf("record[%d]: aliyun configuration is missing", i)
			}
			if record.Aliyun.AccessKeyID == "" || record.Aliyun.AccessKeySecret == "" {
				return fmt.Errorf("record[%d].aliyun credentials cannot be empty", i)
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

// --- Encryption helpers for secrets ---
func ensureKeyFile(keyPath string) ([]byte, error) {
	if keyPath == "" {
		return nil, nil
	}
	if _, err := os.Stat(keyPath); err == nil {
		k, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
		return k, nil
	}
	// create key
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, k, 0600); err != nil {
		return nil, err
	}
	return k, nil
}

func encryptValue(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := append(nonce, ct...)
	return "enc:" + base64.StdEncoding.EncodeToString(out), nil
}

func decryptValue(enc string, key []byte) (string, error) {
	if !strings.HasPrefix(enc, "enc:") {
		return enc, nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, "enc:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce := data[:nonceSize]
	ct := data[nonceSize:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// EncryptConfigSecrets encrypts plaintext secrets in vars and provider fields and writes the config back
// baseDir is used to store/read the key file (.aiolos.key). If baseDir is empty, configDir (dir of configPath) is used.
func EncryptConfigSecrets(cfg *Config, configPath string, baseDir string) (bool, error) {
	if baseDir == "" {
		baseDir = filepath.Dir(configPath)
	}
	keyPath := filepath.Join(baseDir, ".aiolos.key")
	key, err := ensureKeyFile(keyPath)
	if err != nil {
		return false, fmt.Errorf("failed to ensure key file: %w", err)
	}

	changed := false

	// encrypt vars
	if cfg.Vars == nil {
		cfg.Vars = make(map[string]string)
	}
	for k, v := range cfg.Vars {
		if v == "" {
			continue
		}
		// skip already encrypted or env refs or var refs
		if strings.HasPrefix(v, "enc:") || isEnvVarReference(v) || strings.Contains(v, "${var.") {
			continue
		}
		enc, err := encryptValue(v, key)
		if err != nil {
			return false, err
		}
		cfg.Vars[k] = enc
		changed = true
	}

	// encrypt provider secrets
	for i := range cfg.Records {
		r := &cfg.Records[i]
		if r.Cloudflare != nil {
			if r.Cloudflare.APIToken != "" && !strings.HasPrefix(r.Cloudflare.APIToken, "enc:") && !isEnvVarReference(r.Cloudflare.APIToken) && !strings.Contains(r.Cloudflare.APIToken, "${var.") {
				enc, err := encryptValue(r.Cloudflare.APIToken, key)
				if err != nil {
					return false, err
				}
				r.Cloudflare.APIToken = enc
				changed = true
			}
			if r.Cloudflare.ZoneID != "" && !strings.HasPrefix(r.Cloudflare.ZoneID, "enc:") && !isEnvVarReference(r.Cloudflare.ZoneID) && !strings.Contains(r.Cloudflare.ZoneID, "${var.") {
				enc, err := encryptValue(r.Cloudflare.ZoneID, key)
				if err != nil {
					return false, err
				}
				r.Cloudflare.ZoneID = enc
				changed = true
			}
		}
		if r.Aliyun != nil {
			if r.Aliyun.AccessKeyID != "" && !strings.HasPrefix(r.Aliyun.AccessKeyID, "enc:") && !isEnvVarReference(r.Aliyun.AccessKeyID) && !strings.Contains(r.Aliyun.AccessKeyID, "${var.") {
				enc, err := encryptValue(r.Aliyun.AccessKeyID, key)
				if err != nil {
					return false, err
				}
				r.Aliyun.AccessKeyID = enc
				changed = true
			}
			if r.Aliyun.AccessKeySecret != "" && !strings.HasPrefix(r.Aliyun.AccessKeySecret, "enc:") && !isEnvVarReference(r.Aliyun.AccessKeySecret) && !strings.Contains(r.Aliyun.AccessKeySecret, "${var.") {
				enc, err := encryptValue(r.Aliyun.AccessKeySecret, key)
				if err != nil {
					return false, err
				}
				r.Aliyun.AccessKeySecret = enc
				changed = true
			}
		}
	}

	if changed {
		// write back
		if err := WriteConfig(configPath, cfg); err != nil {
			return false, fmt.Errorf("failed to write config after encrypting secrets: %w", err)
		}
	}
	return changed, nil
}
