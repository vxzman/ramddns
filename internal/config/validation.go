package config

import (
	"fmt"
	"net/url"
	"strings"
)

// validateConfig validates the configuration structure and required fields
func validateConfig(cfg *Config) error {
	if len(cfg.Records) == 0 {
		return fmt.Errorf("at least one record must be configured")
	}

	if err := validateIPSource(&cfg.General.GetIP); err != nil {
		return err
	}

	if err := validateProxy(cfg.General.Proxy); err != nil {
		return fmt.Errorf("invalid global proxy: %w", err)
	}

	for i, record := range cfg.Records {
		if err := validateRecord(&record, i, cfg.General.Proxy); err != nil {
			return err
		}
	}

	return nil
}

func validateIPSource(getIP *IPSource) error {
	hasInterface := getIP.Interface != ""
	hasURL := len(getIP.URLs) > 0
	if !hasInterface && !hasURL {
		return fmt.Errorf("either 'get_ip.interface' or 'get_ip.urls' must be configured")
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
	if record.Record == "" {
		return fmt.Errorf("record[%d]: record name is required", index)
	}

	// Validate proxy setting
	if record.UseProxy && globalProxy == "" {
		return fmt.Errorf("record[%d]: use_proxy is true but no global proxy configured", index)
	}
	if record.UseProxy && record.Provider != "cloudflare" {
		return fmt.Errorf("record[%d]: use_proxy only supported for Cloudflare", index)
	}

	// Validate provider-specific configuration
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
	if record.Cloudflare == nil {
		return fmt.Errorf("record[%d]: cloudflare configuration is missing", index)
	}
	if record.Cloudflare.APIToken == "" {
		return fmt.Errorf("record[%d]: cloudflare.api_token is required", index)
	}
	return nil
}

func validateAliyunRecord(record *RecordConfig, index int) error {
	if record.Aliyun == nil {
		return fmt.Errorf("record[%d]: aliyun configuration is missing", index)
	}
	if record.Aliyun.AccessKeyID == "" {
		return fmt.Errorf("record[%d]: aliyun.access_key_id is required", index)
	}
	if record.Aliyun.AccessKeySecret == "" {
		return fmt.Errorf("record[%d]: aliyun.access_key_secret is required", index)
	}
	return nil
}

// validateConfigExpanded validates configuration after secret resolution
func validateConfigExpanded(cfg *Config) error {
	for i, record := range cfg.Records {
		switch record.Provider {
		case "cloudflare":
			if record.Cloudflare.APIToken == "" {
				return fmt.Errorf("record[%d]: cloudflare.api_token is not set or empty", i)
			}
		case "aliyun":
			if record.Aliyun.AccessKeyID == "" {
				return fmt.Errorf("record[%d]: aliyun.access_key_id is not set or empty", i)
			}
			if record.Aliyun.AccessKeySecret == "" {
				return fmt.Errorf("record[%d]: aliyun.access_key_secret is not set or empty", i)
			}
		}
	}
	return nil
}
