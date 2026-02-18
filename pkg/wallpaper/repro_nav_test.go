package wallpaper

import (
	"image"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNavigationConsistency(t *testing.T) {
	rand.Seed(12345) // Fixed seed for reproducibility

	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)

	wp := &Plugin{
		os:            mockOS,
		cfg:           cfg,
		manager:       mockPM,
		downloadedDir: t.TempDir(),
		store:         NewImageStore(),
		Monitors:      make(map[int]*MonitorController),
		runOnUI:       func(f func()) { f() },
	}
	wp.store.SetAsyncSave(false)
	wp.fm = NewFileManager(wp.downloadedDir)
	_ = wp.fm.EnsureDirs()
	wp.store.SetFileManager(wp.fm, wp.downloadedDir+"/cache.json")

	// Setup 3 Images
	for i := 1; i <= 3; i++ {
		name := "img" + string(rune('0'+i))
		path := filepath.Join(wp.downloadedDir, name+".jpg")
		_ = os.WriteFile(path, []byte("dummy"), 0644)
		wp.store.Add(provider.Image{
			ID:              name,
			Path:            "http://example.com/" + name + ".jpg",
			FilePath:        path,
			DerivativePaths: map[string]string{"1920x1080": name + ".jpg"},
		})
	}

	// Mocks
	mockOS.On("GetMonitors").Return([]Monitor{{ID: 0, Rect: image.Rect(0, 0, 1920, 1080)}}, nil)
	mockOS.On("Stat", mock.Anything).Return(nil, nil)
	mockOS.On("SetWallpaper", mock.Anything, 0).Return(nil)
	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	mockPM.On("RefreshTrayMenu").Return()
	mockPM.On("GetAssetManager").Return(&asset.Manager{})

	wp.monitorMenu = make(map[int]*MonitorMenuItems)
	wp.monitorMenu[0] = &MonitorMenuItems{ProviderMenuItem: &fyne.MenuItem{}, ArtistMenuItem: &fyne.MenuItem{}}

	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(0, Monitor{ID: 0, Rect: image.Rect(0, 0, 1920, 1080)}, wp.store, wp.fm, mockOS, cfg, mockIP)
	wp.Monitors[0] = mc

	pump := func() {
		select {
		case cmd := <-mc.Commands:
			mc.handleCommand(cmd)
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Ensure Sequential Mode for simplicity first
	wp.SetShuffleImage(false)
	pump()

	// 1. Initial State
	wp.SetNextWallpaper(-1, true)
	pump()
	// Should be img1 (ID order: img1, img2, img3)
	assert.Equal(t, "img1", mc.State.CurrentImage.ID)

	// 2. Next -> img2
	wp.SetNextWallpaper(-1, true)
	pump()
	assert.Equal(t, "img2", mc.State.CurrentImage.ID)

	// 3. Prev -> img1
	wp.SetPreviousWallpaper(0, true)
	pump()
	assert.Equal(t, "img1", mc.State.CurrentImage.ID)

	// 4. Next -> Should be img2 again?
	// CURRENT BUG: It likely skips to img3 or wraps if RandomPos wasn't decremented
	wp.SetNextWallpaper(-1, true)
	pump()

	// ASSERTION: Ensure consistency
	// If RandomPos was 2 (pointing to img3) and didn't decrement on prev(),
	// next() will pick img3.
	if mc.State.CurrentImage.ID != "img2" {
		t.Fatalf("Inconsistent navigation! Expected img2, got %s. (RandomPos likely not adjusted)", mc.State.CurrentImage.ID)
	}

	// === 5. Shuffle Consistency Test ===
	t.Log("Testing Shuffle Consistency...")

	// Reset to start fresh
	wp.SetShuffleImage(true)
	pump() // Consume UpdateShuffle

	// We don't know the order, but we can verify consistency.
	// Sequence: Next(A) -> Next(B) -> Prev -> Next(Should be B)

	// A
	wp.SetNextWallpaper(-1, true)
	pump()
	idA := mc.State.CurrentImage.ID

	// B
	wp.SetNextWallpaper(-1, true)
	pump()
	idB := mc.State.CurrentImage.ID

	// Check different (unless random collide, but size 3 makes collide unlikely for first 2 if shuffle works well?
	// actually with size 3, rand.Shuffle can put same image? No, it permutes.)
	// Wait, if size=1, A=B. But we have 3.

	// Prev -> Back to A
	wp.SetPreviousWallpaper(0, true)
	pump()
	idBackA := mc.State.CurrentImage.ID
	assert.Equal(t, idA, idBackA, "Prev should take us back to A")

	// Next -> Should be B
	wp.SetNextWallpaper(-1, true)
	pump()
	idFwdB := mc.State.CurrentImage.ID
	assert.Equal(t, idB, idFwdB, "Next after Prev should take us back to B")

	// === 6. Loop Consistency Test (Sequential) ===
	t.Log("Testing Loop Consistency (Sequential)...")
	wp.SetShuffleImage(false)
	pump()

	// We are at some state. Let's fast forward to known state.
	// Actually, easier to just recreate or reset.
	// Let's just Next() 3 times and check ID.
	// Since we strictly follow IDs: img1, img2, img3.
	// We need to find where we are.

	// Force set to img3? No direct way.
	// Just loop until img3.
	count := 0
	for mc.State.CurrentImage.ID != "img3" && count < 10 {
		wp.SetNextWallpaper(-1, true)
		pump()
		count++
	}
	assert.Equal(t, "img3", mc.State.CurrentImage.ID, "Should be at img3")

	// Next -> Should loop to img1
	wp.SetNextWallpaper(-1, true)
	pump()
	assert.Equal(t, "img1", mc.State.CurrentImage.ID, "Should loop back to img1")

	// Prev -> Should loop back to img3
	wp.SetPreviousWallpaper(0, true)
	pump()
	assert.Equal(t, "img3", mc.State.CurrentImage.ID, "Prev should loop back to img3")
}
