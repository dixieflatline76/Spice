package wallpaper

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPauseLogic(t *testing.T) {
	// Setup
	wp := &Plugin{
		store:       &MockImageStore{},
		manager:     &MockPluginManager{},
		cfg:         GetConfig(NewMockPreferences()),
		Monitors:    make(map[int]*MonitorController),
		monitorMenu: make(map[int]*MonitorMenuItems),
		runOnUI:     func(f func()) { f() },
	}

	ms := wp.store.(*MockImageStore)
	mm := wp.manager.(*MockPluginManager)

	// Mock storage and UI
	ms.On("GetUpdateChannel").Return(make(<-chan struct{})).Maybe()
	ms.On("GetIDsForResolution", mock.Anything).Return([]string{"img1", "img2"}).Maybe()
	ms.On("GetByID", mock.Anything).Return(provider.Image{
		ID:              "img1",
		Provider:        "test",
		DerivativePaths: map[string]string{"0x0": "some/path"},
	}, true).Maybe()
	ms.On("MarkSeen", mock.Anything).Return().Maybe()
	mm.On("NotifyUser", mock.Anything, mock.Anything).Return().Maybe()
	mm.On("RefreshTrayMenu").Return().Maybe()
	mm.On("GetAssetManager").Return(asset.NewManager()).Maybe()

	// Create Monitor Controller
	osMock := &MockOS{}
	mc := NewMonitorController(0, Monitor{ID: 0}, ms, nil, osMock, wp.cfg, nil)
	wp.Monitors[0] = mc

	// Mock OS calls
	osMock.On("Stat", mock.Anything).Return(nil, nil).Maybe()
	osMock.On("SetWallpaper", mock.Anything, mock.Anything).Return(nil).Maybe()

	mc.OnWallpaperChanged = func(img provider.Image, monitorID int) {
		wp.updateTrayMenuUI(img, monitorID)
	}

	// Set mock items for UI refresh
	mItems := &MonitorMenuItems{
		PauseMenuItem:    &fyne.MenuItem{Label: "Initial"},
		ProviderMenuItem: &fyne.MenuItem{Label: "Initial"},
		ArtistMenuItem:   &fyne.MenuItem{Label: "Initial"},
		FavoriteMenuItem: &fyne.MenuItem{Label: "Initial"},
	}
	wp.monitorMenu[0] = mItems

	// Start the actor
	mc.Start()
	defer mc.Stop()

	// 1. Initial State: Unpaused
	assert.False(t, mc.State.Paused)

	// 2. Toggle Pause
	wp.TogglePauseMonitorAction(0)

	// Wait for actor to process CmdPause
	time.Sleep(200 * time.Millisecond)

	assert.True(t, mc.State.Paused, "Monitor should be paused after toggle")
	mm.AssertCalled(t, "NotifyUser", "Wallpaper Rotation", "Display 1: Pausing Play")
	assert.Equal(t, "Resume Play", mItems.PauseMenuItem.Label)

	// 3. Test Automatic Transition skipping
	// Give mc a current image
	mc.mu.Lock()
	mc.State.CurrentImage = provider.Image{ID: "initial"}
	mc.mu.Unlock()

	mc.Commands <- CmdNextAuto
	time.Sleep(200 * time.Millisecond)

	mc.mu.RLock()
	currentID := mc.State.CurrentImage.ID
	mc.mu.RUnlock()
	assert.Equal(t, "initial", currentID, "Automatic transition should be skipped when paused")

	// 4. Test Manual Transition proceeding
	mc.Commands <- CmdNext
	time.Sleep(200 * time.Millisecond)

	mc.mu.RLock()
	currentID = mc.State.CurrentImage.ID
	mc.mu.RUnlock()
	assert.NotEqual(t, "initial", currentID, "Manual transition should NOT be skipped when paused")

	// 5. Toggle Resume
	wp.TogglePauseMonitorAction(0)
	time.Sleep(200 * time.Millisecond)
	assert.False(t, mc.State.Paused, "Monitor should be unpaused after second toggle")
	mm.AssertCalled(t, "NotifyUser", "Wallpaper Rotation", "Display 1: Resuming Play")
	assert.Equal(t, "Pause Play", mItems.PauseMenuItem.Label)
}

func TestGlobalPauseToggle(t *testing.T) {
	// Setup
	wp := &Plugin{
		manager:  &MockPluginManager{},
		cfg:      GetConfig(NewMockPreferences()),
		Monitors: make(map[int]*MonitorController),
		runOnUI:  func(f func()) { f() },
	}
	mm := wp.manager.(*MockPluginManager)
	mm.On("NotifyUser", mock.Anything, mock.Anything).Return().Maybe()

	// Create 2 Monitors
	osMock := &MockOS{}
	ms := &MockImageStore{}
	ms.On("GetUpdateChannel").Return((<-chan struct{})(make(chan struct{}))).Maybe()
	mc1 := NewMonitorController(0, Monitor{ID: 0}, ms, nil, osMock, wp.cfg, nil)
	mc2 := NewMonitorController(1, Monitor{ID: 1}, ms, nil, osMock, wp.cfg, nil)
	wp.Monitors[0] = mc1
	wp.Monitors[1] = mc2

	// Start Actors
	mc1.Start()
	mc2.Start()
	defer mc1.Stop()
	defer mc2.Stop()

	// Initial State: Both unpaused
	assert.False(t, wp.IsMonitorPaused(0))
	assert.False(t, wp.IsMonitorPaused(1))

	// Global Pause
	wp.TogglePauseMonitorAction(-1)
	time.Sleep(200 * time.Millisecond) // Wait for actors to process
	assert.True(t, wp.IsMonitorPaused(0), "Monitor 0 should be paused")
	assert.True(t, wp.IsMonitorPaused(1), "Monitor 1 should be paused")

	// Global Resume
	wp.TogglePauseMonitorAction(-1)
	time.Sleep(200 * time.Millisecond) // Wait for actors to process
	assert.False(t, wp.IsMonitorPaused(0), "Monitor 0 should be unpaused")
	assert.False(t, wp.IsMonitorPaused(1), "Monitor 1 should be unpaused")
}
