package wallpaper

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
)

func TestApplyWallpaper_RollbackOnFailure(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)
	mockIP := new(MockImageProcessorTyped)

	wp := &Plugin{
		os:            mockOS,
		imgProcessor:  mockIP,
		cfg:           cfg,
		manager:       mockPM,
		downloadedDir: t.TempDir(),
		store:         NewImageStore(),
		runOnUI:       func(f func()) { f() }, // Run synchronously
		currentIndex:  -1,
	}

	// Mock Asset Manager
	mockAM := asset.NewManager()
	mockPM.On("GetAssetManager").Return(mockAM)
	mockPM.On("RefreshTrayMenu").Return()

	// Init menu items
	wp.providerMenuItem = &fyne.MenuItem{Label: "Initial"}
	wp.artistMenuItem = &fyne.MenuItem{Label: "Initial"}

	// Define Images
	// Define Images
	// Use Real Temp Files for os.Stat to pass
	tempDir := t.TempDir()
	img1Path := filepath.Join(tempDir, "img1.jpg")
	img2Path := filepath.Join(tempDir, "img2.jpg")
	assert.NoError(t, os.WriteFile(img1Path, []byte("dummy"), 0644))
	assert.NoError(t, os.WriteFile(img2Path, []byte("dummy"), 0644))

	img1 := provider.Image{ID: "img1", FilePath: img1Path, Provider: "Prov1", Attribution: "Artist1"}
	img2 := provider.Image{ID: "img2", FilePath: img2Path, Provider: "Prov2", Attribution: "Artist2"}

	// Set Initial State (img1 active)
	wp.currentImage = img1
	wp.store.Add(img1)
	wp.store.Add(img2)

	mockOS.On("SetWallpaper", img2Path).Return(assert.AnError)

	// Action: Apply Wallpaper img2
	wp.applyWallpaper(img2)

	// Assert: OS was called
	mockOS.AssertCalled(t, "SetWallpaper", img2Path)

	// Assert: UI rolled back to img1
	// Since runOnUI is synchronous, the final state of the menu items should match img1
	assert.Equal(t, "Source: Prov1", wp.providerMenuItem.Label)
	assert.Equal(t, "By: Artist1", wp.artistMenuItem.Label)

	// Check RefershTrayMenu was called (likely twice: once for optimistic, once for rollback)
	// We can't easily count exact calls with strict order without detailed mock checking,
	// but ensuring it was called is basic sanity.
	mockPM.AssertCalled(t, "RefreshTrayMenu")
}
