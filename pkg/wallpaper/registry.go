package wallpaper

import (
	"net/http"

	"github.com/dixieflatline76/Spice/pkg/provider"
)

// ProviderFactory defines the function signature for creating a provider.
type ProviderFactory func(cfg *Config, client *http.Client) provider.ImageProvider

var providerRegistry = make(map[string]ProviderFactory)

// RegisterProvider registers a new image provider factory.
func RegisterProvider(name string, factory ProviderFactory) {
	providerRegistry[name] = factory
}

// GetRegisteredProviders returns all registered provider factories.
func GetRegisteredProviders() map[string]ProviderFactory {
	return providerRegistry
}
