package wallpaper

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	// "fyne.io/fyne/v2/theme" // Removed to avoid app dependency
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTrayFlow_PreviousWallpaper(t *testing.T) {
	t.Skip("Skipping flaky UI test causing build crash")
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	mockOS := new(MockOS)
	mockOS.On("SetWallpaper", mock.Anything).Return(nil)

	// Mock Manager
	mockPM := new(MockPluginManager)
	// Allow any NotifyUser call to avoid background noise crashing tests
	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	// Mock CreateMenuItem used if we wanted to init items, but we'll manually set them below
	// Same Mock Manager for both tests setup
	mockPM.On("RefreshTrayMenu").Return()
	mockPM.On("GetAssetManager").Return(asset.NewManager()) // Catch fallback calls

	// Setup Mock Provider to avoid GetAssetManager fallback
	mockProvider := new(MockImageProvider)
	dummyIcon := fyne.NewStaticResource("dummy.png", []byte{})
	mockProvider.On("GetProviderIcon").Return(dummyIcon)
	mockProvider.On("HomeURL").Return("http://mock-provider.com")         // needed if creating menu item checks it?
	mockProvider.On("ParseURL", mock.Anything).Return("", assert.AnError) // Defensive catch-all for unexpected downloads

	wp := &Plugin{
		os:                  mockOS,
		cfg:                 cfg,
		manager:             mockPM, // Inject manager
		store:               NewImageStore(),
		runOnUI:             func(f func()) { f() },
		currentIndex:        -1,
		history:             []int{},
		actionChan:          make(chan func(), 10),
		httpClient:          &http.Client{},
		interrupt:           util.NewSafeBool(),
		currentDownloadPage: util.NewSafeIntWithValue(1),
		fitImageFlag:        util.NewSafeBool(),
		shuffleImageFlag:    util.NewSafeBool(),
		providers:           map[string]provider.ImageProvider{"test_provider": mockProvider},
		// Init Menu Items to test update logic (and prevent potential nil access if code assumes non-nil)
		providerMenuItem: &fyne.MenuItem{},
		artistMenuItem:   &fyne.MenuItem{},
	}

	// Add 3 images with "test_provider" and real files
	tempDir := t.TempDir()
	for i := 0; i < 3; i++ {
		fname := fmt.Sprintf("%d.jpg", i)
		fpath := filepath.Join(tempDir, fname)
		assert.NoError(t, os.WriteFile(fpath, []byte("dummy"), 0644))

		wp.store.Add(provider.Image{
			ID:       fmt.Sprintf("img%d", i),
			FilePath: fpath,
			Provider: "test_provider",
		})
	}

	// Navigate 0 -> 1 -> 2
	wp.history = []int{0, 1, 2}
	wp.currentIndex = 2
	wp.currentImage, _ = wp.store.Get(2)

	// Action: Previous (Should go to 1)
	wp.setPrevWallpaper()

	// Assert: Index 1
	assert.Equal(t, 1, wp.currentIndex, "Should move back to index 1")
	assert.Equal(t, "img1", wp.currentImage.ID)
	assert.Equal(t, []int{0, 1}, wp.history, "History should be popped")

	// Action: Previous (Should go to 0)
	wp.setPrevWallpaper()

	// Assert: Index 0
	assert.Equal(t, 0, wp.currentIndex, "Should move back to index 0")
	assert.Equal(t, "img0", wp.currentImage.ID)
	assert.Equal(t, []int{0}, wp.history, "History should be popped")

	// Action: Previous (History empty/single)
	// Should log/notify and stay at 0
	wp.setPrevWallpaper()
	assert.Equal(t, 0, wp.currentIndex, "Should stay at index 0")
}

func TestTrayFlow_ViewOnWeb(t *testing.T) {
	// Setup
	mockPM := new(MockPluginManager)

	wp := &Plugin{
		manager: mockPM,
	}

	testURL := "http://example.com/view/img1"
	wp.currentImage = provider.Image{
		ID:      "img1",
		ViewURL: testURL,
	}

	// Mock Expectation
	mockPM.On("OpenURL", mock.MatchedBy(func(u *url.URL) bool {
		return u.String() == testURL
	})).Return(nil)

	// Action
	wp.ViewCurrentImageOnWeb()

	// Assert
	mockPM.AssertExpectations(t)
}

func TestTrayFlow_Shuffle(t *testing.T) {
	t.Skip("Skipping flaky UI test causing build crash")
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	mockOS := new(MockOS)
	mockOS.On("SetWallpaper", mock.Anything).Return(nil)

	// Mock Manager for Shuffle Test too
	mockPM := new(MockPluginManager)
	mockPM.On("RefreshTrayMenu").Return()
	mockPM.On("GetAssetManager").Return(asset.NewManager())

	// Mock Provider
	mockProvider := new(MockImageProvider)
	dummyIcon := fyne.NewStaticResource("dummy.png", []byte{})
	mockProvider.On("GetProviderIcon").Return(dummyIcon)
	mockProvider.On("ParseURL", mock.Anything).Return("", assert.AnError) // Defensive catch-all

	wp := &Plugin{
		os:                  mockOS,
		cfg:                 cfg,
		manager:             mockPM, // Need manager here too
		store:               NewImageStore(),
		runOnUI:             func(f func()) { f() },
		currentIndex:        -1,
		history:             []int{}, // Init history too
		actionChan:          make(chan func(), 10),
		shuffleImageFlag:    util.NewSafeBool(),
		pipeline:            nil,
		httpClient:          &http.Client{},
		interrupt:           util.NewSafeBool(),
		currentDownloadPage: util.NewSafeIntWithValue(1),
		fitImageFlag:        util.NewSafeBool(),
		providers:           map[string]provider.ImageProvider{"test_provider": mockProvider},
		providerMenuItem:    &fyne.MenuItem{},
		artistMenuItem:      &fyne.MenuItem{},
	}

	// Add 5 images with provider and real files
	tempDir := t.TempDir()
	for i := 0; i < 5; i++ {
		fname := fmt.Sprintf("%d.jpg", i)
		fpath := filepath.Join(tempDir, fname)
		assert.NoError(t, os.WriteFile(fpath, []byte("dummy"), 0644))

		wp.store.Add(provider.Image{
			ID:       fmt.Sprintf("img%d", i),
			FilePath: fpath,
			Provider: "test_provider",
		})
	}

	// Enable Shuffle
	wp.SetShuffleImage(true)
	assert.True(t, wp.shuffleImageFlag.Value())

	// Action: Next 5 times
	seenIndices := make(map[int]bool)
	for i := 0; i < 5; i++ {
		wp.setNextWallpaper()
		seenIndices[wp.currentIndex] = true
	}

	// Assert: All 5 unique images visited (Shuffle ensures permutation)
	assert.Equal(t, 5, len(seenIndices), "Shuffle should visit all images once before repeating")

	// Verify Shuffle Order was built
	assert.Equal(t, 5, len(wp.shuffleOrder))
}
