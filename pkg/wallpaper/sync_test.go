//go:build !linux

package wallpaper

import (
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
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
	mc.State.CurrentID = img.ID
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
	mockPM.On("GetAssetManager").Return(nil).Maybe()

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

	menu := wp.CreateTrayMenu()
	found := false
	for _, item := range menu.Items {
		if item.Label == "By: Artist" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected 'By: Artist' in tray menu items")
}
