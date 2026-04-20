// Package provider defines the interface for DNS providers
package provider

import "context"

// Provider is the interface that all DNS providers must implement
type Provider interface {
	// Name returns the provider name
	Name() string

	// UpsertRecord creates or updates a DNS record
	// Returns true if the record was successfully updated/created
	UpsertRecord(ctx context.Context, zone, record, ip string, ttl int, extra map[string]interface{}) (bool, error)
}

// ProviderFactory creates a new provider instance
type ProviderFactory func(config map[string]interface{}) (Provider, error)

// RecordExtra represents provider-specific extra parameters
type RecordExtra map[string]interface{}

// ProxySupporter is an optional interface for providers that support proxy
type ProxySupporter interface {
	// SetProxy sets the proxy URL for the provider
	SetProxy(proxyURL string) error

	// GetProxy returns the current proxy URL
	GetProxy() string
}
