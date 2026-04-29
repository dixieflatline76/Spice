package wallpaper

import (
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// SyncProviders triggers synchronization for all providers that support it (both user queries and remote configs).
func (wp *Plugin) SyncProviders() {
	for name, p := range wp.providers {
		// Sync User Queries (Wallhaven, etc)
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

		// Sync Remote Configs (Museum Curated Lists, etc)
		if rcSyncer, ok := p.(provider.RemoteConfigSyncer); ok {
			log.Debugf("Triggering remote config sync for provider: %s", name)
			go func(s provider.RemoteConfigSyncer, providerName string) {
				if err := s.SyncRemoteConfig(); err != nil {
					log.Debugf("Remote config sync failed for provider %s: %v", providerName, err)
				} else {
					log.Debugf("Remote config sync completed for provider %s", providerName)
				}
			}(rcSyncer, name)
		}
	}
}
