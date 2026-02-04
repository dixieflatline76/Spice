package wallpaper

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSyncMonitors_RefreshUI(t *testing.T) {
	// Setup
	prefs := NewMemoryPreferences()
	wp := setupTestPlugin(t, prefs)
	mockPM := wp.manager.(*MockPluginManager)
	mockOS := wp.os.(*MockOS)

	// Clear default expectations from setupTestPlugin so we can set our own sticky ones
	mockPM.ExpectedCalls = nil

	// Setup Initial State: Monitor 0 exists and has an image
	img := provider.Image{ID: "test_img", Provider: "stub", Attribution: "Artist"}
	wp.store.Add(img)

	// Pre-populate monitor controller
	// We need to simulate that the monitor was already there
	// SyncMonitors will see it in "current" list and "wp.Monitors" list

	mc := NewMonitorController(0, Monitor{ID: 0}, wp.store, wp.fm, wp.os, wp.cfg, wp.imgProcessor)
	mc.State.CurrentImage = img
	wp.Monitors[0] = mc

	// Expectation: updateTrayMenuUI calls RefreshTrayMenu
	// The sync logic calls updateTrayMenuUI in a goroutine
	// updateTrayMenuUI calls manager.RefreshTrayMenu()

	// We need to wait for the goroutine, so we use a channel or WaitGroup?
	// The mock can block or signal
	refreshCalled := make(chan struct{}, 1)
	mockPM.On("RefreshTrayMenu").Run(func(args mock.Arguments) {
		select {
		case refreshCalled <- struct{}{}:
		default:
		}
	}).Return()

	// also allow GetAssetManager which is called by updateTrayMenuUI
	mockPM.On("GetAssetManager").Return(nil).Maybe() // mock asset manager return is checked for nil in code?
	// The code calls wp.manager.GetAssetManager().GetIcon("favorite.png") if favorite menu item exists
	// But in this test, monitorMenu map is empty, so updateTrayMenuUI returns early!

	// Crucial: We need to populate wp.monitorMenu so updateTrayMenuUI proceeds
	wp.monitorMenu = make(map[int]*MonitorMenuItems)
	wp.monitorMenu[0] = &MonitorMenuItems{
		ProviderMenuItem: &fyne.MenuItem{},
		ArtistMenuItem:   &fyne.MenuItem{},
	}

	// Stub OS.GetMonitors to return the SAME monitor
	mockOS.ExpectedCalls = nil // Clear setup calls
	mockOS.On("GetMonitors").Return([]Monitor{{ID: 0, Name: "Primary"}}, nil)

	// Action: Run Sync (Force=true to ensure logic runs even if count matches)
	wp.SyncMonitors(true)

	// Verify
	select {
	case <-refreshCalled:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for RefreshTrayMenu to be called")
	}

	assert.Equal(t, "By: Artist", wp.monitorMenu[0].ArtistMenuItem.Label)
}
