package wallpaper

import (
	"context"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// SyncWallhavenCollections triggers synchronization for all providers that support it.
func (wp *Plugin) SyncWallhavenCollections() {
	for name, p := range wp.providers {
		if syncer, ok := p.(provider.Syncer); ok {
			log.Printf("Triggering automated sync for provider: %s", name)
			go func(s provider.Syncer, providerName string) {
				if err := s.Sync(context.Background()); err != nil {
					log.Printf("Automated sync failed for provider %s: %v", providerName, err)
				} else {
					log.Printf("Automated sync completed for provider %s", providerName)
				}
			}(syncer, name)
		}
	}
}
