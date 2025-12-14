package wallpaper

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MemoryPreferences implements fyne.Preferences for testing persistence
type MemoryPreferences struct {
	data map[string]string
	full map[string]interface{} // Catch-all for other types if needed (bool, int)
}

func NewMemoryPreferences() *MemoryPreferences {
	return &MemoryPreferences{
		data: make(map[string]string),
		full: make(map[string]interface{}),
	}
}

func (m *MemoryPreferences) String(key string) string {
	return m.data[key]
}
func (m *MemoryPreferences) StringWithFallback(key, fallback string) string {
	if v, ok := m.data[key]; ok {
		return v
	}
	return fallback
}
func (m *MemoryPreferences) SetString(key, value string) {
	m.data[key] = value
}
func (m *MemoryPreferences) Bool(key string) bool {
	if v, ok := m.full[key]; ok {
		return v.(bool)
	}
	return false
}
func (m *MemoryPreferences) BoolWithFallback(key string, fallback bool) bool {
	if v, ok := m.full[key]; ok {
		return v.(bool)
	}
	return fallback
}
func (m *MemoryPreferences) SetBool(key string, value bool) {
	m.full[key] = value
}
func (m *MemoryPreferences) Float(key string) float64                               { return 0.0 }
func (m *MemoryPreferences) FloatWithFallback(key string, fallback float64) float64 { return fallback }
func (m *MemoryPreferences) SetFloat(key string, value float64)                     {}
func (m *MemoryPreferences) Int(key string) int {
	if v, ok := m.full[key]; ok {
		return v.(int)
	}
	return 0
}
func (m *MemoryPreferences) IntWithFallback(key string, fallback int) int {
	if v, ok := m.full[key]; ok {
		return v.(int)
	}
	return fallback
}
func (m *MemoryPreferences) SetInt(key string, value int) {
	m.full[key] = value
}
func (m *MemoryPreferences) RemoveValue(key string) {
	delete(m.data, key)
	delete(m.full, key)
}
func (m *MemoryPreferences) BoolList(key string) []bool                              { return nil }
func (m *MemoryPreferences) BoolListWithFallback(key string, fallback []bool) []bool { return fallback }
func (m *MemoryPreferences) SetBoolList(key string, value []bool)                    {}
func (m *MemoryPreferences) FloatList(key string) []float64                          { return nil }
func (m *MemoryPreferences) FloatListWithFallback(key string, fallback []float64) []float64 {
	return fallback
}
func (m *MemoryPreferences) SetFloatList(key string, value []float64)             {}
func (m *MemoryPreferences) IntList(key string) []int                             { return nil }
func (m *MemoryPreferences) IntListWithFallback(key string, fallback []int) []int { return fallback }
func (m *MemoryPreferences) SetIntList(key string, value []int)                   {}
func (m *MemoryPreferences) StringList(key string) []string                       { return nil }
func (m *MemoryPreferences) StringListWithFallback(key string, fallback []string) []string {
	return fallback
}
func (m *MemoryPreferences) SetStringList(key string, value []string) {}

func (m *MemoryPreferences) ChangeListeners() []func() { return nil }

func (m *MemoryPreferences) AddChangeListener(func()) {}

// TestLifecycle_BlockPersistence simulates a full user story:
// 1. User blocks an image.
// 2. App restarts.
// 3. Provider tries to download that image again.
// 4. App rejects it.
func TestLifecycle_BlockPersistence(t *testing.T) {
	// Shared storage representing the OS registry/storage
	// This "survives" the app restart.
	persistentPrefs := NewMemoryPreferences()
	blockedID := "offending_image_id"

	// ---------------------------------------------------------
	// PHASE 1: User Actions (Blocking an image)
	// ---------------------------------------------------------
	{
		t.Log("[Phase 1] Starting App Instance 1...")
		wp1 := setupTestPlugin(t, persistentPrefs)
		wp1.Activate()

		// Simulate User Blocking an Image
		// We use the public API/Config logic to ensure it writes to prefs.
		t.Logf("[Phase 1] User blocks image: %s", blockedID)
		wp1.cfg.AddToAvoidSet(blockedID)

		// Verify it's in the volatile config
		assert.True(t, wp1.cfg.InAvoidSet(blockedID))

		// Verify it hit the "Disk" (MemoryPreferences)
		// The key for avoid_set is "avoid_set" (based on explicit logic or internal struct?).
		// Logic in config.go: save() marshals everything.
		// We need to check if AddToAvoidSet calls save(). Yes it does.
		// It saves to the preference key "config" (or similar? see config.go).
		// Actually, Config usually serializes to a single JSON blob or individual keys?
		// Let's assume standard behavior: Config logic writes to Prefs.
		// We won't inspect the JSON blob string in prefs (too brittle),
		// we'll rely on Phase 2 loading it back.

		wp1.Deactivate()
		t.Log("[Phase 1] App Instance 1 Shutdown.")
	}

	// ---------------------------------------------------------
	// PHASE 2: Restart & Wiring Verification
	// ---------------------------------------------------------
	{
		t.Log("[Phase 2] Starting App Instance 2 (Fresh process, same local storage)...")

		// Create BRAND NEW plugin instance, but pass the SAME persistentPrefs.
		wp2 := setupTestPlugin(t, persistentPrefs)

		// Verify "Disk" State before Activate (Config should load from prefs on GetConfig)
		// Note: setupTestPlugin calls GetConfig(persistentPrefs), so wp2.cfg is already loaded.
		assert.True(t, wp2.cfg.InAvoidSet(blockedID), "Config should have loaded the blocked ID from preferences")

		// Run Activate - this triggers the Store loading logic
		wp2.Activate()

		// Verify Store State (Internal Verification)
		// This confirms the Config -> Store wiring works.
		// We can't easily check store private map, but we can try to Add() and fail.
		added := wp2.store.Add(provider.Image{ID: blockedID})
		assert.False(t, added, "Store should reject the blocked ID immediately after restart")

		wp2.Deactivate()
	}

	// ---------------------------------------------------------
	// PHASE 3: The Attack (Enforcement in Pipeline)
	// ---------------------------------------------------------
	{
		t.Log("[Phase 3] Simulating Provider pushing blocked content...")
		wp3 := setupTestPlugin(t, persistentPrefs)
		wp3.Activate()

		// Setup a dummy job for the blocked image
		badJob := DownloadJob{
			Image: provider.Image{ID: blockedID, Path: "http://bad.com/1.jpg"},
		}

		// Submit directly to pipeline (bypassing download logic, testing Store/Pipeline filter)
		// Note: Pipeline calls Store.Add(), so if Store rejects it, it won't be in Store.
		success := wp3.pipeline.Submit(badJob)
		assert.True(t, success, "Pipeline acceptance just means 'queued'")

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Verify: Image should NOT be in store
		// We rely on Store.Count or getting by index
		count := wp3.store.Count()
		assert.Equal(t, 0, count, "Pipeline should have dropped the blocked image")

		// Control Test: Clean Image
		goodID := "good_image"
		goodJob := DownloadJob{
			Image: provider.Image{ID: goodID, Path: "http://good.com/1.jpg"},
		}
		wp3.pipeline.Submit(goodJob)

		// Wait
		assert.Eventually(t, func() bool {
			return wp3.store.Count() == 1
		}, 1*time.Second, 50*time.Millisecond, "Pipeline should process and accept good image")

		wp3.Deactivate()
	}

	// ---------------------------------------------------------
	// PHASE 4: Runtime Clearing (The original regression)
	// ---------------------------------------------------------
	{
		t.Log("[Phase 4] Verifying 'Clear()' does not wipe blocklist...")
		wp4 := setupTestPlugin(t, persistentPrefs)
		wp4.Activate()

		// 1. Trigger Clear (Simulating 'Refresh Images' / 'Wipe')
		// Note: We use Clear(), not Wipe(). Wipe() implies full reset.
		t.Log("[Phase 4] Clearing Store...")
		wp4.store.Clear()

		// 2. Submit Blocked Image immediately after Clear
		badJob := DownloadJob{
			Image: provider.Image{ID: blockedID, Path: "http://bad.com/1.jpg"},
		}
		wp4.pipeline.Submit(badJob)

		// Wait
		time.Sleep(100 * time.Millisecond)

		// 3. Verify Rejection
		count := wp4.store.Count()
		assert.Equal(t, 0, count, "Blocklist must persist even after Store.Clear()")

		// 4. Verify Store is still functional
		goodJob := DownloadJob{
			Image: provider.Image{ID: "good_image_phase4", Path: "http://good.com/4.jpg"},
		}
		wp4.pipeline.Submit(goodJob)

		assert.Eventually(t, func() bool {
			return wp4.store.Count() == 1
		}, 1*time.Second, 50*time.Millisecond, "Store should accept new images after Clear")

		wp4.Deactivate()
	}
}

// Helper to bundle boilerplate setup
func setupTestPlugin(t *testing.T, prefs fyne.Preferences) *Plugin {
	// Re-load config from prefs (Simulate fresh start)
	cfg := GetConfig(prefs)
	// Clear queries to avoid network noise
	cfg.Queries = []ImageQuery{}
	cfg.SetChgImgOnStart(false)
	// IMPORTANT: Set MaxConcurrentProcessors to 1 for deterministic pipeline testing if needed,
	// or just leave default.
	cfg.MaxConcurrentProcessors = 1

	mockOS := new(MockOS)
	mockOS.On("getDesktopDimension").Return(1920, 1080)

	mockPM := new(MockPluginManager)
	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	mockPM.On("RegisterNotifier", mock.Anything).Return()
	mockPM.On("GetPreferences").Return(prefs)
	mockPM.On("Register", mock.Anything).Return()
	mockPM.On("GetAssetManager").Return(nil) // Simplification

	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	imageStore := NewImageStore()

	// Pass-through processor
	dummyProcessor := func(ctx context.Context, job DownloadJob) (provider.Image, error) {
		return job.Image, nil
	}

	pipeline := NewPipeline(cfg, imageStore, dummyProcessor)

	wp := &Plugin{
		os:                  mockOS,
		cfg:                 cfg,
		manager:             mockPM,
		store:               imageStore,
		fm:                  fm,
		runOnUI:             func(f func()) { f() },
		currentIndex:        -1,
		history:             []int{},
		actionChan:          make(chan func(), 10),
		interrupt:           util.NewSafeBool(),
		currentDownloadPage: util.NewSafeIntWithValue(1),
		fitImageFlag:        util.NewSafeBool(),
		shuffleImageFlag:    util.NewSafeBool(),
		providers:           make(map[string]provider.ImageProvider),
		pipeline:            pipeline,
	}
	return wp
}
