package wallpaper

import (
	"log"
)

// MigrationFunc defines a single migration step.
// It returns true if the configuration was modified.
type MigrationFunc func(*Config) (bool, error)

// MigrationChain manages a sequence of migration steps.
type MigrationChain struct {
	Steps []MigrationFunc
}

// NewMigrationChain creates a new chain with the standard migration order.
func NewMigrationChain() *MigrationChain {
	return &MigrationChain{
		Steps: []MigrationFunc{
			EnsureFavoritesStep,
			LoadAvoidSetStep,
			BackfillWallhavenStep,
			BackfillPexelsStep,
			UnifyQueriesStep,
			BackfillUnifiedStep,
			SanitizeFaceSettingsStep,
			PruneStaleFavoritesStep,
			EnsureManagedFavoritesStep,
			UpdateCollisionsStep,
		},
	}
}

// Execute benchmarks the chain and saves if any changes occurred.
func (mc *MigrationChain) Execute(cfg *Config) error {
	anyChanged := false
	for _, step := range mc.Steps {
		changed, err := step(cfg)
		if err != nil {
			return err
		}
		if changed {
			anyChanged = true
		}
	}

	if anyChanged {
		cfg.save()
	}
	return nil
}

// EnsureFavoritesStep ensures the Favorites query exists using the correct ID.
func EnsureFavoritesStep(cfg *Config) (bool, error) {
	hasFavorites := false
	for _, q := range cfg.Queries {
		if q.ID == FavoritesQueryID {
			hasFavorites = true
			break
		}
	}
	if !hasFavorites {
		log.Println("Migration: Adding missing Favorites query to configuration.")
		favQuery := ImageQuery{
			ID:          FavoritesQueryID,
			Description: "Favorite Images",
			URL:         FavoritesQueryID,
			Active:      true,
			Provider:    "Favorites",
			Managed:     true, // Ensure new Favorites are managed
		}
		cfg.Queries = append(cfg.Queries, favQuery)
		return true, nil
	}
	return false, nil
}

// LoadAvoidSetStep populates the sync.Map from the loaded AvoidSet map.
// It also cleans up legacy un-namespaced IDs (e.g. "24645") that cause
// cross-provider collisions. Only properly namespaced IDs (containing "_")
// are kept.
func LoadAvoidSetStep(cfg *Config) (bool, error) {
	if cfg.AvoidSet == nil {
		return false, nil
	}
	changed := false
	for k := range cfg.AvoidSet {
		// Only purge strictly numeric legacy IDs (e.g. "24645").
		// These caused collisions between different providers in older versions.
		// Modern namespaced IDs and local file paths are spared.
		isLegacyNumeric := true
		for _, char := range k {
			if char < '0' || char > '9' {
				isLegacyNumeric = false
				break
			}
		}

		if isLegacyNumeric {
			log.Printf("Migration: Removing legacy un-namespaced numeric blocklist entry: %s", k)
			delete(cfg.AvoidSet, k)
			changed = true
			continue
		}
		// Hydrate avoidMap as we go (Redundant but safe for direct step callers)
		cfg.avoidMap.Store(k, true)
	}
	return changed, nil
}

// BackfillWallhavenStep generates IDs for legacy Wallhaven queries.
func BackfillWallhavenStep(cfg *Config) (bool, error) {
	changed := false
	for i, q := range cfg.ImageQueries {
		if q.ID == "" {
			cfg.ImageQueries[i].ID = GenerateQueryID(q.URL)
			changed = true
		}
	}
	return changed, nil
}

// BackfillPexelsStep generates IDs for legacy Pexels queries.
func BackfillPexelsStep(cfg *Config) (bool, error) {
	changed := false
	for i, q := range cfg.PexelsQueries {
		if q.ID == "" {
			cfg.PexelsQueries[i].ID = GenerateQueryID(q.URL)
			changed = true
		}
	}
	return changed, nil
}

// UnifyQueriesStep merges legacy query lists into the main Queries list.
func UnifyQueriesStep(cfg *Config) (bool, error) {
	if len(cfg.ImageQueries) > 0 || len(cfg.PexelsQueries) > 0 {
		log.Print("Migrating legacy queries to unified list...")
		for _, q := range cfg.ImageQueries {
			q.Provider = "Wallhaven"
			cfg.Queries = append(cfg.Queries, q)
		}
		for _, q := range cfg.PexelsQueries {
			q.Provider = "Pexels"
			cfg.Queries = append(cfg.Queries, q)
		}

		// Clear legacy lists
		cfg.ImageQueries = make([]ImageQuery, 0)
		cfg.PexelsQueries = make([]ImageQuery, 0)
		return true, nil
	}
	return false, nil
}

// BackfillUnifiedStep ensures IDs and Providers are set for all queries in the unified list.
func BackfillUnifiedStep(cfg *Config) (bool, error) {
	changed := false
	for i, q := range cfg.Queries {
		if q.ID == "" {
			cfg.Queries[i].ID = GenerateQueryID(q.URL)
			changed = true
		}
		if q.Provider == "" {
			cfg.Queries[i].Provider = "Wallhaven"
			changed = true
		}
	}
	return changed, nil
}

// SanitizeFaceSettingsStep ensures Face Boost and Face Crop are not both enabled.
func SanitizeFaceSettingsStep(cfg *Config) (bool, error) {
	faceCrop := cfg.BoolWithFallback(FaceCropPrefKey, false)
	faceBoost := cfg.BoolWithFallback(FaceBoostPrefKey, false)

	if faceCrop && faceBoost {
		cfg.SetBool(FaceBoostPrefKey, false)
		// This modifies preference store directly, but doesn't necessarily dirty the struct fields unless we re-read.
		// However, loadFromPrefs doesn't set struct fields for these prefs, getters read directly.
		// So this is a side-effect. We return false because `cfg.save()` might not save preferences, only struct JSON?
		// Wait, `cfg.save()` marshals `Config` to JSON string and stores in `wallhavenConfigPrefKey`.
		// Face settings are separate keys.
		// So `cfg.save()` won't help persist Face settings changes if they are separate prefs.
		// But `SetBool` saves immediately to Fyne prefs.
		return false, nil
	}
	return false, nil
}

// PruneStaleFavoritesStep removes legacy "Favorites" queries with incorrect IDs.
func PruneStaleFavoritesStep(cfg *Config) (bool, error) {
	var finalQueries []ImageQuery
	changed := false

	for _, q := range cfg.Queries {
		if q.Provider == "Favorites" && q.ID != FavoritesQueryID {
			log.Printf("Migration: Pruning stale Favorites query: %s", q.ID)
			changed = true
			continue
		}
		finalQueries = append(finalQueries, q)
	}

	if changed {
		cfg.Queries = finalQueries
	}
	return changed, nil
}

// UpdateCollisionsStep ensures no ID collisions by regenerating IDs with provider prefix.
func UpdateCollisionsStep(cfg *Config) (bool, error) {
	changed := false
	for i := range cfg.Queries {
		if cfg.Queries[i].Provider == "Favorites" {
			continue
		}
		// Generate new ID using provider + URL
		newID := GenerateQueryID(cfg.Queries[i].Provider + ":" + cfg.Queries[i].URL)
		if cfg.Queries[i].ID != newID {
			log.Printf("Migration: Updating query ID for %s: %s -> %s", cfg.Queries[i].Provider, cfg.Queries[i].ID, newID)
			cfg.Queries[i].ID = newID
			changed = true
		}
	}
	return changed, nil
}

// EnsureManagedFavoritesStep ensures existing Favorites queries are marked as Managed.
func EnsureManagedFavoritesStep(cfg *Config) (bool, error) {
	changed := false
	for i := range cfg.Queries {
		if cfg.Queries[i].Provider == "Favorites" && !cfg.Queries[i].Managed {
			log.Printf("Migration: Marking Favorites query as managed: %s", cfg.Queries[i].ID)
			cfg.Queries[i].Managed = true
			changed = true
		}
	}
	return changed, nil
}
