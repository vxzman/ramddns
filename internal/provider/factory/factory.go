// Package factory provides DNS provider factory functions
package factory

import (
	"fmt"

	"ramddns/internal/config"
	"ramddns/internal/provider"
	"ramddns/internal/provider/aliyun"
	"ramddns/internal/provider/cloudflare"
)

// ProviderRegistry maps provider names to factory functions
type ProviderRegistry map[string]ProviderFactory

// ProviderFactory creates a new provider instance
type ProviderFactory func(cfg *config.Config, record *config.RecordConfig) (provider.Provider, error)

// registry holds all registered provider factories
var registry = ProviderRegistry{
	"cloudflare": newCloudflareProvider,
	"aliyun":     newAliyunProvider,
}

// Register registers a new provider factory
func Register(name string, factory ProviderFactory) {
	registry[name] = factory
}

// GetProvider creates a provider instance based on the provider name
func GetProvider(cfg *config.Config, record *config.RecordConfig) (provider.Provider, error) {
	factory, ok := registry[record.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", record.Provider)
	}
	return factory(cfg, record)
}

// ListProviders returns a list of registered provider names
func ListProviders() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// IsSupported checks if a provider is supported
func IsSupported(providerName string) bool {
	_, ok := registry[providerName]
	return ok
}

// newCloudflareProvider creates a new Cloudflare provider
func newCloudflareProvider(cfg *config.Config, record *config.RecordConfig) (provider.Provider, error) {
	if record.APIToken == "" {
		return nil, fmt.Errorf("cloudflare api_token is missing")
	}

	proxyURL := config.GetRecordProxy(cfg, record)
	providerConfig := &cloudflare.SimpleConfig{
		Proxy: proxyURL,
	}

	return cloudflare.NewProvider(providerConfig, record.APIToken), nil
}

// newAliyunProvider creates a new Aliyun provider
func newAliyunProvider(cfg *config.Config, record *config.RecordConfig) (provider.Provider, error) {
	if record.AccessKeyID == "" || record.AccessKeySecret == "" {
		return nil, fmt.Errorf("aliyun access_key_id or access_key_secret is missing")
	}

	return aliyun.NewProvider(record.AccessKeyID, record.AccessKeySecret), nil
}
