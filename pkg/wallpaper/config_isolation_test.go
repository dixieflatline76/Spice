package wallpaper

import (
	"encoding/json"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSerializationIsolation(t *testing.T) {
	// Goal: Prove that primitive toggles are completely invisible to the JSON marshaler.
	prefs := test.NewApp().Preferences()
	mgr := &asset.Manager{}
	cfg := &Config{
		Preferences: prefs,
		assetMgr:    mgr,
	}

	// 1. Set primitive toggles via native setters
	cfg.SetShortcutsDisabled(true)
	cfg.SetTargetedShortcutsDisabled(true)
	cfg.SetWallhavenSyncEnabled(true)

	// 2. Trigger a save
	cfg.save()

	// 3. Inspect the stored JSON blob in Fyne Preferences
	jsonStr := prefs.String(WallhavenConfigPrefKey)
	require.NotEmpty(t, jsonStr, "JSON config blob should not be empty after save")

	var rawMap map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &rawMap)
	require.NoError(t, err, "Should be able to unmarshal the saved JSON")

	// 4. Assert that isolated keys strictly do NOT exist in the map
	restrictedKeys := []string{
		"shortcuts_disabled",
		"targeted_shortcuts_disabled",
		"wallhaven_sync_enabled",
		"wallhaven_api_key",
		"wallhaven_username",
		"logLevel",
		"maxConcurrentProcessors",
	}

	for _, key := range restrictedKeys {
		_, exists := rawMap[key]
		assert.False(t, exists, "Key '%s' should NOT exist in the JSON blob (it must be isolated)", key)
	}
}

func TestAntiClobberStateSurvival(t *testing.T) {
	// Goal: Prove that complex data saves do not clobber native primitive states.
	prefs := test.NewApp().Preferences()
	mgr := &asset.Manager{}
	cfg := &Config{
		Preferences: prefs,
		assetMgr:    mgr,
	}

	// 1. Set TargetedShortcutsDisabled to true (Native state)
	cfg.SetTargetedShortcutsDisabled(true)
	require.True(t, cfg.GetTargetedShortcutsDisabled())

	// 2. Add a new dummy ImageQuery (triggers c.save() and internal state sync)
	_, err := cfg.AddImageQuery("Anti-Clobber Test", "https://example.com/api", true)
	require.NoError(t, err)

	// 3. Assert it still returns true
	// Previously, the stale struct field (default false) would have been marshaled into JSON,
	// then potentially re-loaded or shadowed during the process.
	assert.True(t, cfg.GetTargetedShortcutsDisabled(), "TargetedShortcutsDisabled must survive a JSON save/sync cycle")

	// Double check Wallhaven Sync too
	cfg.SetWallhavenSyncEnabled(true)
	_, _ = cfg.AddImageQuery("Another Query", "https://example.com/2", true)
	assert.True(t, cfg.GetWallhavenSyncEnabled(), "WallhavenSyncEnabled must survive a JSON save/sync cycle")
}

func TestNativeIORoundTrip(t *testing.T) {
	// Goal: Prove that setters write to Fyne and getters read from Fyne, persisting across instances.
	prefs := test.NewApp().Preferences() // Shared mock storage
	mgr := &asset.Manager{}

	// Instance A
	cfgA := &Config{
		Preferences: prefs,
		assetMgr:    mgr,
	}
	cfgA.SetTargetedShortcutsDisabled(false)
	cfgA.SetShortcutsDisabled(true)

	// Instance B (Fresh instance, same storage)
	cfgB := &Config{
		Preferences: prefs,
		assetMgr:    mgr,
	}

	assert.False(t, cfgB.GetTargetedShortcutsDisabled(), "Fresh instance MUST read false from storage")
	assert.True(t, cfgB.GetShortcutsDisabled(), "Fresh instance MUST read true from storage")

	// Complex check: change in B, read in A
	cfgB.SetShortcutsDisabled(false)
	assert.False(t, cfgA.GetShortcutsDisabled(), "Instance A MUST see changes made by Instance B via shared storage")
}
