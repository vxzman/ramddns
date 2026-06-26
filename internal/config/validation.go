package config

import (
	"fmt"
	"net/url"
	"strings"
)

// validateConfig validates the configuration structure and required fields.
func validateConfig(cfg *Config) error {
	if len(cfg.Records) == 0 {
		return fmt.Errorf("at least one record must be configured")
	}

	if err := validateIPSource(&cfg.IPSource); err != nil {
		return err
	}

	if err := validateProxy(cfg.Proxy); err != nil {
		return fmt.Errorf("invalid proxy: %w", err)
	}

	for i, record := range cfg.Records {
		if err := validateRecord(&record, i, cfg.Proxy); err != nil {
			return err
		}
	}

	return nil
}

func validateIPSource(ipSource *IPSource) error {
	hasInterface := ipSource.Interface != ""
	hasURLs := len(ipSource.FallbackURLs) > 0
	if !hasInterface && !hasURLs {
		return fmt.Errorf("either 'ip_source.interface' or 'ip_source.fallback_urls' must be configured")
	}
	return nil
}

func validateProxy(proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("proxy must include scheme (e.g., 'socks5://', 'http://')")
	}
	scheme := strings.ToLower(u.Scheme)
	if !isValidProxyScheme(scheme) {
		return fmt.Errorf("unsupported proxy scheme '%s'", scheme)
	}
	return nil
}

func isValidProxyScheme(scheme string) bool {
	valid := map[string]bool{
		"http": true, "https": true,
		"socks5": true, "socks5h": true,
	}
	return valid[scheme]
}

func validateRecord(record *RecordConfig, index int, globalProxy string) error {
	if record.Provider == "" {
		return fmt.Errorf("record[%d]: provider is required", index)
	}
	if record.Zone == "" {
		return fmt.Errorf("record[%d]: zone is required", index)
	}
	if record.Name == "" {
		return fmt.Errorf("record[%d]: name is required", index)
	}

	// Validate proxy setting
	if record.UseProxy && globalProxy == "" {
		return fmt.Errorf("record[%d]: use_proxy is true but no global proxy configured", index)
	}
	if record.UseProxy && record.Provider != "cloudflare" {
		return fmt.Errorf("record[%d]: use_proxy only supported for Cloudflare", index)
	}

	// Validate provider-specific fields
	switch record.Provider {
	case "cloudflare":
		return validateCloudflareRecord(record, index)
	case "aliyun":
		return validateAliyunRecord(record, index)
	default:
		return fmt.Errorf("record[%d]: unsupported provider '%s'", index, record.Provider)
	}
}

func validateCloudflareRecord(record *RecordConfig, index int) error {
	if record.APIToken == "" {
		return fmt.Errorf("record[%d]: api_token is required for Cloudflare", index)
	}
	return nil
}

func validateAliyunRecord(record *RecordConfig, index int) error {
	if record.AccessKeyID == "" {
		return fmt.Errorf("record[%d]: access_key_id is required for Aliyun", index)
	}
	if record.AccessKeySecret == "" {
		return fmt.Errorf("record[%d]: access_key_secret is required for Aliyun", index)
	}
	return nil
}

// validateConfigExpanded validates configuration after secret resolution.
func validateConfigExpanded(cfg *Config) error {
	for i, record := range cfg.Records {
		switch record.Provider {
		case "cloudflare":
			if record.APIToken == "" {
				return fmt.Errorf("record[%d]: api_token is not set or empty", i)
			}
		case "aliyun":
			if record.AccessKeyID == "" {
				return fmt.Errorf("record[%d]: access_key_id is not set or empty", i)
			}
			if record.AccessKeySecret == "" {
				return fmt.Errorf("record[%d]: access_key_secret is not set or empty", i)
			}
		}
	}
	return nil
}
