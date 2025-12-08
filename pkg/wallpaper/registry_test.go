package wallpaper

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderRegistry(t *testing.T) {
	// 1. Verify all expected providers are registered
	providers := GetRegisteredProviders()
	expectedProviders := []string{"Wallhaven", "Unsplash", "Pexels"}

	for _, name := range expectedProviders {
		assert.Contains(t, providers, name, "Provider %s should be registered", name)
	}

	// 2. Verify factory instantiation
	cfg := &Config{}
	client := &http.Client{}

	for name, factory := range providers {
		provider := factory(cfg, client)
		assert.NotNil(t, provider, "Factory for %s should return a provider", name)
		assert.Equal(t, name, provider.Name(), "Provider name should match key")
	}
}
