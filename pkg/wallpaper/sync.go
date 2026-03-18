package wallpaper

import (
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// SyncWallhavenCollections triggers synchronization for all providers that support it.
func (wp *Plugin) SyncWallhavenCollections() {
	for name, p := range wp.providers {
		if syncer, ok := p.(provider.Syncer); ok {
			log.Debugf("Triggering automated sync for provider: %s", name)
			go func(s provider.Syncer, providerName string) {
				if err := s.Sync(wp.ctx); err != nil {
					log.Debugf("Automated sync failed for provider %s: %v", providerName, err)
				} else {
					log.Debugf("Automated sync completed for provider %s", providerName)
				}
			}(syncer, name)
		}
	}
}
