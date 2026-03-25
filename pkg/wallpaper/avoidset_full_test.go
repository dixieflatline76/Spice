package wallpaper

import (
	"image"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	util_log "github.com/dixieflatline76/Spice/v2/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func init() {
	// Disable debug logging in tests to avoid environment-specific logger issues
	util_log.SetDebugEnabled(false)
}

// helper to create a correctly initialized controller for testing
func newTestMC_AvoidSet(id int, cfg *Config, store StoreInterface) *MonitorController {
	mockOS := &MockOS{}
	mockOS.On("SetWallpaper", mock.Anything, mock.Anything).Return(nil)
	mockOS.On("Stat", mock.Anything).Return(nil, nil) // Satisfy Stat checks for temp files

	return &MonitorController{
		ID:    id,
		cfg:   cfg,
		Store: store,
		Monitor: Monitor{
			Rect: image.Rect(0, 0, 1920, 1080),
		},
		State: &MonitorState{},
		os:    mockOS,
	}
}

func TestAvoidSet_Persistence_FullCycle(t *testing.T) {
	// 1. Initial Setup: Create isolated config and add IDs
	prefs := NewMockPreferences()
	u, _ := user.Current()
	cfg := &Config{
		Preferences: prefs,
		AvoidSet:    make(map[string]bool),
		userid:      u.Uid,
		assetMgr:    asset.NewManager(),
	}
	// Initial load to get defaults
	cfg.loadFromPrefs()

	localID := "LocalFolder_abc123_photo"
	wallhavenID := "Wallhaven_456"
	legacyID := "789" // 4. Update AvoidSet

	cfg.AddToAvoidSet(localID)
	cfg.AddToAvoidSet(wallhavenID)
	cfg.AddToAvoidSet(legacyID)

	// Wait for async save (with retry/poll for robustness on CI)
	var prefJSON string
	success := false
	for i := 0; i < 20; i++ {
		prefJSON = prefs.String(wallhavenConfigPrefKey)
		if strings.Contains(prefJSON, localID) && strings.Contains(prefJSON, wallhavenID) {
			success = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.True(t, success, "Preference must contain the IDs before reloading. JSON: %s", prefJSON)

	// 2. Simulate Restart: Load fresh isolated config from the same shared preferences
	cfg2 := &Config{
		Preferences: prefs,
		AvoidSet:    make(map[string]bool),
		userid:      u.Uid,
		assetMgr:    asset.NewManager(),
	}
	err := cfg2.loadFromPrefs()
	require.NoError(t, err)

	// 3. Verify
	assert.True(t, cfg2.InAvoidSet(localID), "Local namespaced ID must survive")
	assert.True(t, cfg2.InAvoidSet(wallhavenID), "Online namespaced ID must survive")
	assert.False(t, cfg2.InAvoidSet(legacyID), "Legacy numeric ID should have been purged")
}

// TestAvoidSet_SessionWide_Enforcement verifies that blocking an image on one monitor
// causes other monitors to skip it immediately.
func TestAvoidSet_SessionWide_Enforcement(t *testing.T) {
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	store := NewImageStore()

	// Create real temp files to satisfy Stat checks
	tmpDir := t.TempDir()
	path1 := filepath.Join(tmpDir, "img1.jpg")
	path2 := filepath.Join(tmpDir, "img2.jpg")
	require.NoError(t, os.WriteFile(path1, []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(path2, []byte("fake"), 0644))

	// Add two images to store with resolution buckets
	img1 := provider.Image{
		ID:              "image1",
		DerivativePaths: map[string]string{"1920x1080": path1},
	}
	img2 := provider.Image{
		ID:              "image2",
		DerivativePaths: map[string]string{"1920x1080": path2},
	}
	store.Add(img1)
	store.Add(img2)

	// Use helper to ensure MC is fully initialized
	mc1 := newTestMC_AvoidSet(1, cfg, store)
	mc1.State.ShuffleIDs = []string{"image1", "image2"}
	mc1.State.RandomPos = 0

	mc2 := newTestMC_AvoidSet(2, cfg, store)
	mc2.State.ShuffleIDs = []string{"image1", "image2"}
	mc2.State.RandomPos = 0

	// 1. MC1 blocks 'image1'
	mc1.State.CurrentImage = img1
	mc1.deleteCurrent() // This adds image1 to AvoidSet and removes from store

	// 2. Verify MC2 skips it immediately
	mc2.next(true)
	assert.Equal(t, "image2", mc2.State.CurrentID, "Monitor 2 should have skipped blocked image1 and picked image2")
}

// TestAvoidSet_Shuffle_Recovery verifies the retry loop when multiple images are blocked.
func TestAvoidSet_Shuffle_Recovery(t *testing.T) {
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	store := NewImageStore()

	// Create real temp file to satisfy Stat check
	tmpDir := t.TempDir()
	path3 := filepath.Join(tmpDir, "img3.jpg")
	require.NoError(t, os.WriteFile(path3, []byte("fake"), 0644))

	// Create a sequence: [Blocked, Blocked, Valid]
	img3 := provider.Image{
		ID:              "valid",
		DerivativePaths: map[string]string{"1920x1080": path3},
	}

	// We only add 'valid' to the store, and block the others
	store.Add(img3)
	cfg.AddToAvoidSet("blocked1")
	cfg.AddToAvoidSet("blocked2")

	// Use helper to ensure MC is fully initialized
	mc := newTestMC_AvoidSet(1, cfg, store)
	mc.State.ShuffleIDs = []string{"blocked1", "blocked2", "valid"}
	mc.State.RandomPos = 0

	// Trigger next
	mc.next(true)

	// Should have looped through blocked1, blocked2 and landed on valid
	assert.Equal(t, "valid", mc.State.CurrentID, "Should have recovered and found the single valid image")
	assert.False(t, mc.State.WaitingForImages, "Should NOT be waiting for images if recovery found one")
}
