package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Record:   "www",
						TTL:      180,
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_CLOUDFLARE_API_TOKEN}",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid_aliyun_config",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						URLs: []string{"https://ipv6.icanhazip.com"},
					},
				},
				Records: []RecordConfig{
					{
						Provider: "aliyun",
						Zone:     "example.cn",
						Record:   "dev",
						TTL:      600,
						Aliyun: &AliyunRecord{
							AccessKeyID:     "${TEST_ALIYUN_ACCESS_KEY_ID}",
							AccessKeySecret: "${TEST_ALIYUN_ACCESS_KEY_SECRET}",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty_records",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{},
			},
			wantErr: true,
			errMsg:  "at least one record must be configured",
		},
		{
			name: "missing_ip_source",
			cfg: &Config{
				General: GeneralConfig{},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Record:   "www",
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_CLOUDFLARE_API_TOKEN}",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "either 'get_ip.interface' or 'get_ip.urls' must be configured",
		},
		{
			name: "missing_provider",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Zone:   "example.com",
						Record: "www",
					},
				},
			},
			wantErr: true,
			errMsg:  "provider is required",
		},
		{
			name: "missing_zone",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Record:   "www",
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_CLOUDFLARE_API_TOKEN}",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "zone is required",
		},
		{
			name: "missing_record_name",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_CLOUDFLARE_API_TOKEN}",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "record name is required",
		},
		{
			name: "missing_cloudflare_token",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider:   "cloudflare",
						Zone:       "example.com",
						Record:     "www",
						Cloudflare: &CloudflareRecord{},
					},
				},
			},
			wantErr: true,
			errMsg:  "cloudflare.api_token is empty",
		},
		{
			name: "missing_aliyun_credentials",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "aliyun",
						Zone:     "example.cn",
						Record:   "www",
						Aliyun:   &AliyunRecord{},
					},
				},
			},
			wantErr: true,
			errMsg:  "aliyun.access_key_id is empty",
		},
		{
			name: "unsupported_provider",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "unknown",
						Zone:     "example.com",
						Record:   "www",
					},
				},
			},
			wantErr: true,
			errMsg:  "unsupported provider",
		},
		{
			name: "invalid_proxy_url",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
					Proxy: "invalid-proxy",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Record:   "www",
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_CLOUDFLARE_API_TOKEN}",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid global proxy",
		},
		{
			name: "use_proxy_without_global_proxy",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Record:   "www",
						UseProxy: true,
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_CLOUDFLARE_API_TOKEN}",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "use_proxy is true but no global proxy configured",
		},
		{
			name: "valid_proxy_url",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
					Proxy: "socks5://127.0.0.1:1080",
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Record:   "www",
						UseProxy: true,
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_CLOUDFLARE_API_TOKEN}",
						},
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
			err := validateProxyURL(tt.proxy)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProxyURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetCacheFilePath(t *testing.T) {
	tests := []struct {
		name      string
		configFile string
		workDir   string
		wantContains string
	}{
		{
			name:      "with_work_dir",
			configFile: "/etc/ipflow/config.json",
			workDir:   "/var/lib/ipflow",
			wantContains: "cache.lastip",
		},
		{
			name:      "without_work_dir",
			configFile: "/etc/ipflow/config.json",
			workDir:   "",
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
	// 创建临时文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.lastip")

	// 测试空文件
	got := ReadLastIP(testFile)
	if got != "" {
		t.Errorf("ReadLastIP() for non-existent file = %v, want empty", got)
	}

	// 测试有内容的文件
	testIP := "2001:db8::1"
	if err := os.WriteFile(testFile, []byte(testIP), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	got = ReadLastIP(testFile)
	if got != testIP {
		t.Errorf("ReadLastIP() = %v, want %v", got, testIP)
	}

	// 测试带换行符的内容
	testIPWithNewline := testIP + "\n"
	if err := os.WriteFile(testFile, []byte(testIPWithNewline), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	got = ReadLastIP(testFile)
	if got != testIP {
		t.Errorf("ReadLastIP() with newline = %v, want %v", got, testIP)
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

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	if string(content) != testIP {
		t.Errorf("WriteLastIP() wrote %v, want %v", string(content), testIP)
	}
}

func TestGetRecordProxy(t *testing.T) {
	cfg := &Config{
		General: GeneralConfig{
			Proxy: "socks5://127.0.0.1:1080",
		},
	}

	tests := []struct {
		name     string
		record   *RecordConfig
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
		name     string
		record   *RecordConfig
		wantTTL  int
	}{
		{
			name: "cloudflare_with_record_ttl",
			record: &RecordConfig{
				Provider: "cloudflare",
				TTL:      300,
				Cloudflare: &CloudflareRecord{
					APIToken: "test",
				},
			},
			wantTTL: 300,
		},
		{
			name: "cloudflare_without_record_ttl",
			record: &RecordConfig{
				Provider: "cloudflare",
				Cloudflare: &CloudflareRecord{
					APIToken: "test",
				},
			},
			wantTTL: 180, // default for cloudflare
		},
		{
			name: "aliyun_with_record_ttl",
			record: &RecordConfig{
				Provider: "aliyun",
				TTL:      600,
				Aliyun: &AliyunRecord{
					AccessKeyID:     "test",
					AccessKeySecret: "test",
				},
			},
			wantTTL: 600,
		},
		{
			name: "aliyun_without_record_ttl",
			record: &RecordConfig{
				Provider: "aliyun",
				Aliyun: &AliyunRecord{
					AccessKeyID:     "test",
					AccessKeySecret: "test",
				},
			},
			wantTTL: 600, // default for aliyun
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

	// 设置测试环境变量
	t.Setenv("TEST_CLOUDFLARE_API_TOKEN", "test_token_12345678901234567890")

	// 测试有效配置
	validConfig := `{
		"general": {
			"get_ip": {
				"interface": "eth0"
			}
		},
		"records": [
			{
				"provider": "cloudflare",
				"zone": "example.com",
				"record": "www",
				"cloudflare": {
					"api_token": "${TEST_CLOUDFLARE_API_TOKEN}"
				}
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

	// 测试无效 JSON
	invalidJSON := `{ invalid json }`
	invalidFile := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(invalidFile, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, _ = ReadConfig(invalidFile, true)
	if cfg != nil {
		t.Errorf("ReadConfig() for invalid JSON should return nil")
	}

	// 测试不存在的文件
	cfg, _ = ReadConfig(filepath.Join(tmpDir, "nonexistent.json"), true)
	if cfg != nil {
		t.Errorf("ReadConfig() for non-existent file should return nil")
	}
}

func TestExpandConfigEnvVars(t *testing.T) {
	// 设置测试环境变量
	t.Setenv("TEST_API_TOKEN", "test_token_12345678901234567890")
	t.Setenv("TEST_ACCESS_KEY_ID", "LTAItest1234567890")
	t.Setenv("TEST_ACCESS_KEY_SECRET", "test_secret_1234567890")
	t.Setenv("TEST_ZONE_ID", "zone123xyz")

	tests := []struct {
		name     string
		cfg      *Config
		verifyFn func(*testing.T, *Config)
	}{
		{
			name: "expand_cloudflare_env_vars",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Record:   "www",
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_API_TOKEN}",
							ZoneID:   "${TEST_ZONE_ID}",
						},
					},
				},
			},
			verifyFn: func(t *testing.T, cfg *Config) {
				if cfg.Records[0].Cloudflare.APIToken != "test_token_12345678901234567890" {
					t.Errorf("APIToken = %v, want test_token_12345678901234567890", cfg.Records[0].Cloudflare.APIToken)
				}
				if cfg.Records[0].Cloudflare.ZoneID != "zone123xyz" {
					t.Errorf("ZoneID = %v, want zone123xyz", cfg.Records[0].Cloudflare.ZoneID)
				}
			},
		},
		{
			name: "expand_aliyun_env_vars",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "aliyun",
						Zone:     "example.cn",
						Record:   "dev",
						Aliyun: &AliyunRecord{
							AccessKeyID:     "${TEST_ACCESS_KEY_ID}",
							AccessKeySecret: "${TEST_ACCESS_KEY_SECRET}",
						},
					},
				},
			},
			verifyFn: func(t *testing.T, cfg *Config) {
				if cfg.Records[0].Aliyun.AccessKeyID != "LTAItest1234567890" {
					t.Errorf("AccessKeyID = %v, want LTAItest1234567890", cfg.Records[0].Aliyun.AccessKeyID)
				}
				if cfg.Records[0].Aliyun.AccessKeySecret != "test_secret_1234567890" {
					t.Errorf("AccessKeySecret = %v, want test_secret_1234567890", cfg.Records[0].Aliyun.AccessKeySecret)
				}
			},
		},
		{
			name: "env_with_default_value",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Record:   "www",
						Cloudflare: &CloudflareRecord{
							APIToken: "${TEST_API_TOKEN}",
							ZoneID:   "${NON_EXISTENT_VAR:-default_zone}",
						},
					},
				},
			},
			verifyFn: func(t *testing.T, cfg *Config) {
				// 未设置的变量使用默认值
				if cfg.Records[0].Cloudflare.ZoneID != "default_zone" {
					t.Errorf("ZoneID with default = %v, want default_zone", cfg.Records[0].Cloudflare.ZoneID)
				}
			},
		},
		{
			name: "non_env_values_unchanged",
			cfg: &Config{
				General: GeneralConfig{
					GetIP: IPSource{
						Interface: "eth0",
					},
				},
				Records: []RecordConfig{
					{
						Provider: "cloudflare",
						Zone:     "example.com",
						Record:   "www",
						Cloudflare: &CloudflareRecord{
							APIToken: "static_token",
							ZoneID:   "",
						},
					},
				},
			},
			verifyFn: func(t *testing.T, cfg *Config) {
				if cfg.Records[0].Cloudflare.APIToken != "static_token" {
					t.Errorf("Static APIToken = %v, want static_token", cfg.Records[0].Cloudflare.APIToken)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandConfigEnvVars(tt.cfg)
			tt.verifyFn(t, tt.cfg)
		})
	}
}
