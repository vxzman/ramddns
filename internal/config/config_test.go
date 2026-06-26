package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid_cloudflare_config",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Name:     "www",
						TTL:      180,
						APIToken: "static_token",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_aliyun_config",
			cfg: &Config{
				IPSource: IPSource{
					FallbackURLs: []string{"https://ipv6.icanhazip.com"},
				},
				Records: []RecordConfig{
					{
						Provider:        "aliyun",
						Zone:            "example.cn",
						Name:            "dev",
						TTL:             600,
						AccessKeyID:     "test_id",
						AccessKeySecret: "test_secret",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty_records",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{},
			},
			wantErr: true,
			errMsg:  "at least one record must be configured",
		},
		{
			name: "missing_ip_source",
			cfg: &Config{
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Name:     "www",
						APIToken: "token",
					},
				},
			},
			wantErr: true,
			errMsg:  "either 'ip_source.interface' or 'ip_source.fallback_urls' must be configured",
		},
		{
			name: "missing_provider",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Zone: "example.com",
						Name: "www",
					},
				},
			},
			wantErr: true,
			errMsg:  "provider is required",
		},
		{
			name: "missing_zone",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Name:     "www",
						APIToken: "token",
					},
				},
			},
			wantErr: true,
			errMsg:  "zone is required",
		},
		{
			name: "missing_record_name",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						APIToken: "token",
					},
				},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "missing_cloudflare_token",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Name:     "www",
					},
				},
			},
			wantErr: true,
			errMsg:  "api_token is required for Cloudflare",
		},
		{
			name: "missing_aliyun_credentials",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Provider: "aliyun",
						Zone:     "example.cn",
						Name:     "www",
					},
				},
			},
			wantErr: true,
			errMsg:  "access_key_id is required for Aliyun",
		},
		{
			name: "unsupported_provider",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Provider: "unknown",
						Zone:     "example.com",
						Name:     "www",
					},
				},
			},
			wantErr: true,
			errMsg:  "unsupported provider",
		},
		{
			name: "invalid_proxy_url",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Proxy: "invalid-proxy",
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Name:     "www",
						APIToken: "token",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid proxy",
		},
		{
			name: "use_proxy_without_global_proxy",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Name:     "www",
						UseProxy: true,
						APIToken: "token",
					},
				},
			},
			wantErr: true,
			errMsg:  "use_proxy is true but no global proxy configured",
		},
		{
			name: "valid_proxy_url",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Proxy: "socks5://127.0.0.1:1080",
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Name:     "www",
						UseProxy: true,
						APIToken: "token",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateConfig() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateProxyURL(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{"empty_proxy", "", false},
		{"valid_socks5", "socks5://127.0.0.1:1080", false},
		{"valid_socks5h", "socks5h://127.0.0.1:1080", false},
		{"valid_http", "http://proxy.example.com:8080", false},
		{"valid_https", "https://proxy.example.com:8080", false},
		{"missing_scheme", "127.0.0.1:1080", true},
		{"invalid_scheme", "ftp://proxy.example.com", true},
		{"malformed_url", "not-a-url", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProxy(tt.proxy)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProxy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetCacheFilePath(t *testing.T) {
	tests := []struct {
		name         string
		configFile   string
		workDir      string
		wantContains string
	}{
		{
			name:         "with_work_dir",
			configFile:   "/etc/ramddns/config.json",
			workDir:      "/var/lib/ramddns",
			wantContains: "cache.lastip",
		},
		{
			name:         "without_work_dir",
			configFile:   "/etc/ramddns/config.json",
			workDir:      "",
			wantContains: "cache.lastip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetCacheFilePath(tt.configFile, tt.workDir)
			if got == "" {
				t.Errorf("GetCacheFilePath() returned empty string")
			}
			if tt.wantContains != "" && filepath.Base(got) != tt.wantContains {
				t.Errorf("GetCacheFilePath() = %v, want file containing %v", got, tt.wantContains)
			}
		})
	}
}

func TestReadLastIP(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.lastip")

	got := ReadLastIP(testFile)
	if got != "" {
		t.Errorf("ReadLastIP() for non-existent file = %v, want empty", got)
	}

	testIP := "2001:db8::1"
	ts, _ := time.Parse(time.RFC3339, "2026-05-01T10:30:00Z")
	cacheData := CacheFileData{
		LastIP:  testIP,
		History: []IPHistoryEntry{{Timestamp: ts, IP: testIP}},
	}
	if err := WriteCacheFile(testFile, cacheData); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	got = ReadLastIP(testFile)
	if got != testIP {
		t.Errorf("ReadLastIP() = %v, want %v", got, testIP)
	}
}

func TestWriteLastIP(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.lastip")

	testIP := "2001:db8::1"
	err := WriteLastIP(testFile, testIP)
	if err != nil {
		t.Errorf("WriteLastIP() error = %v", err)
	}

	data := ParseCacheFile(testFile)
	if data.LastIP != testIP {
		t.Errorf("WriteLastIP() lastIP = %v, want %v", data.LastIP, testIP)
	}
	if len(data.History) != 1 || data.History[0].IP != testIP {
		t.Errorf("WriteLastIP() history = %v, want 1 entry with IP %v", data.History, testIP)
	}
}

func TestParseCacheFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.cache")

	data := ParseCacheFile(testFile)
	if data.LastIP != "" {
		t.Errorf("ParseCacheFile() non-existent file LastIP = %v, want empty", data.LastIP)
	}
	if len(data.History) != 0 {
		t.Errorf("ParseCacheFile() non-existent file History len = %v, want 0", len(data.History))
	}

	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	data = ParseCacheFile(testFile)
	if data.LastIP != "" {
		t.Errorf("ParseCacheFile() empty file LastIP = %v, want empty", data.LastIP)
	}

	fileContent := `2026-05-01T10:30:00Z 2001:db8::1
2026-05-02T08:15:00Z 2001:db8::2
2026-05-03T12:00:00Z 2001:db8::3
`
	if err := os.WriteFile(testFile, []byte(fileContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	data = ParseCacheFile(testFile)
	if data.LastIP != "2001:db8::3" {
		t.Errorf("ParseCacheFile() LastIP = %v, want 2001:db8::3 (last entry)", data.LastIP)
	}
	if len(data.History) != 3 {
		t.Errorf("ParseCacheFile() History len = %v, want 3", len(data.History))
	}
	if len(data.History) >= 2 {
		if data.History[0].IP != "2001:db8::1" {
			t.Errorf("History[0].IP = %v, want 2001:db8::1", data.History[0].IP)
		}
		if data.History[1].IP != "2001:db8::2" {
			t.Errorf("History[1].IP = %v, want 2001:db8::2", data.History[1].IP)
		}
		if data.History[2].IP != "2001:db8::3" {
			t.Errorf("History[2].IP = %v, want 2001:db8::3", data.History[2].IP)
		}
	}
}

func TestWriteCacheFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.cache")

	ts1, _ := time.Parse(time.RFC3339, "2026-05-01T10:30:00Z")
	ts2, _ := time.Parse(time.RFC3339, "2026-05-02T08:15:00Z")

	data := CacheFileData{
		LastIP: "2001:db8::1",
		History: []IPHistoryEntry{
			{Timestamp: ts1, IP: "2001:db8::1"},
			{Timestamp: ts2, IP: "2001:db8::2"},
		},
	}

	err := WriteCacheFile(testFile, data)
	if err != nil {
		t.Errorf("WriteCacheFile() error = %v", err)
	}

	readData := ParseCacheFile(testFile)
	if readData.LastIP != "2001:db8::2" {
		t.Errorf("roundtrip LastIP = %v, want 2001:db8::2 (last entry)", readData.LastIP)
	}
	if len(readData.History) != 2 {
		t.Errorf("roundtrip History len = %v, want 2", len(readData.History))
	}
}

func TestAppendIPHistory(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.cache")

	oldIP, err := AppendIPHistory(testFile, "2001:db8::1")
	if err != nil {
		t.Errorf("AppendIPHistory() error = %v", err)
	}
	if oldIP != "" {
		t.Errorf("AppendIPHistory() oldIP = %v, want empty (first write)", oldIP)
	}

	data := ParseCacheFile(testFile)
	if data.LastIP != "2001:db8::1" {
		t.Errorf("After first append LastIP = %v, want 2001:db8::1", data.LastIP)
	}
	if len(data.History) != 1 {
		t.Errorf("After first append History len = %v, want 1", len(data.History))
	}

	oldIP, err = AppendIPHistory(testFile, "2001:db8::2")
	if err != nil {
		t.Errorf("AppendIPHistory() error = %v", err)
	}
	if oldIP != "2001:db8::1" {
		t.Errorf("AppendIPHistory() oldIP = %v, want 2001:db8::1", oldIP)
	}

	data = ParseCacheFile(testFile)
	if data.LastIP != "2001:db8::2" {
		t.Errorf("After second append LastIP = %v, want 2001:db8::2", data.LastIP)
	}
	if len(data.History) != 2 {
		t.Errorf("After second append History len = %v, want 2", len(data.History))
	}
}

func TestGetRecordProxy(t *testing.T) {
	cfg := &Config{
		Proxy: "socks5://127.0.0.1:1080",
	}

	tests := []struct {
		name      string
		record    *RecordConfig
		wantProxy string
	}{
		{
			name: "use_proxy_true",
			record: &RecordConfig{
				UseProxy: true,
			},
			wantProxy: "socks5://127.0.0.1:1080",
		},
		{
			name: "use_proxy_false",
			record: &RecordConfig{
				UseProxy: false,
			},
			wantProxy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRecordProxy(cfg, tt.record)
			if got != tt.wantProxy {
				t.Errorf("GetRecordProxy() = %v, want %v", got, tt.wantProxy)
			}
		})
	}
}

func TestGetRecordTTL(t *testing.T) {
	tests := []struct {
		name    string
		record  *RecordConfig
		wantTTL int
	}{
		{
			name: "cloudflare_with_record_ttl",
			record: &RecordConfig{
				Provider: "cloudflare",
				TTL:      300,
				APIToken: "test",
			},
			wantTTL: 300,
		},
		{
			name: "cloudflare_without_record_ttl",
			record: &RecordConfig{
				Provider: "cloudflare",
				APIToken: "test",
			},
			wantTTL: 180,
		},
		{
			name: "aliyun_with_record_ttl",
			record: &RecordConfig{
				Provider:        "aliyun",
				TTL:             600,
				AccessKeyID:     "test",
				AccessKeySecret: "test",
			},
			wantTTL: 600,
		},
		{
			name: "aliyun_without_record_ttl",
			record: &RecordConfig{
				Provider:        "aliyun",
				AccessKeyID:     "test",
				AccessKeySecret: "test",
			},
			wantTTL: 600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRecordTTL(tt.record)
			if got != tt.wantTTL {
				t.Errorf("GetRecordTTL() = %v, want %v", got, tt.wantTTL)
			}
		})
	}
}

func TestReadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	validConfig := `{
		"ip_source": {
			"interface": "eth0"
		},
		"records": [
			{
				"provider": "cloudflare",
				"zone": "example.com",
				"name": "www",
				"api_token": "test_token"
			}
		]
	}`

	validFile := filepath.Join(tmpDir, "valid.json")
	if err := os.WriteFile(validFile, []byte(validConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, absPath := ReadConfig(validFile, true)
	if cfg == nil {
		t.Errorf("ReadConfig() for valid config returned nil")
	}
	if absPath == "" {
		t.Errorf("ReadConfig() for valid config returned empty absolute path")
	}

	invalidJSON := `{ invalid json }`
	invalidFile := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(invalidFile, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, _ = ReadConfig(invalidFile, true)
	if cfg != nil {
		t.Errorf("ReadConfig() for invalid JSON should return nil")
	}

	cfg, _ = ReadConfig(filepath.Join(tmpDir, "nonexistent.json"), true)
	if cfg != nil {
		t.Errorf("ReadConfig() for non-existent file should return nil")
	}
}

func TestExpandConfigEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *Config
		verifyFn func(*testing.T, *Config)
	}{
		{
			name: "expand_cloudflare_env_vars",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Env: map[string]string{
					"TEST_API_TOKEN": "test_token_12345678901234567890",
					"TEST_ZONE_ID":   "zone123xyz",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Name:     "www",
						APIToken: "$TEST_API_TOKEN",
						ZoneID:   "$TEST_ZONE_ID",
					},
				},
			},
			verifyFn: func(t *testing.T, cfg *Config) {
				if cfg.Records[0].APIToken != "test_token_12345678901234567890" {
					t.Errorf("APIToken = %v, want test_token_12345678901234567890", cfg.Records[0].APIToken)
				}
				if cfg.Records[0].ZoneID != "zone123xyz" {
					t.Errorf("ZoneID = %v, want zone123xyz", cfg.Records[0].ZoneID)
				}
			},
		},
		{
			name: "expand_aliyun_env_vars",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Env: map[string]string{
					"TEST_ACCESS_KEY_ID":     "LTAItest1234567890",
					"TEST_ACCESS_KEY_SECRET": "test_secret_1234567890",
				},
				Records: []RecordConfig{
					{
						Provider:        "aliyun",
						Zone:            "example.cn",
						Name:            "dev",
						AccessKeyID:     "$TEST_ACCESS_KEY_ID",
						AccessKeySecret: "$TEST_ACCESS_KEY_SECRET",
					},
				},
			},
			verifyFn: func(t *testing.T, cfg *Config) {
				if cfg.Records[0].AccessKeyID != "LTAItest1234567890" {
					t.Errorf("AccessKeyID = %v, want LTAItest1234567890", cfg.Records[0].AccessKeyID)
				}
				if cfg.Records[0].AccessKeySecret != "test_secret_1234567890" {
					t.Errorf("AccessKeySecret = %v, want test_secret_1234567890", cfg.Records[0].AccessKeySecret)
				}
			},
		},
		{
			name: "non_env_values_unchanged",
			cfg: &Config{
				IPSource: IPSource{
					Interface: "eth0",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Name:     "www",
						APIToken: "static_token",
					},
				},
			},
			verifyFn: func(t *testing.T, cfg *Config) {
				if cfg.Records[0].APIToken != "static_token" {
					t.Errorf("Static APIToken = %v, want static_token", cfg.Records[0].APIToken)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResolveSecrets(tt.cfg)
			tt.verifyFn(t, tt.cfg)
		})
	}
}
