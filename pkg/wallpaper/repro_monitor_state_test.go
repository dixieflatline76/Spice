package wallpaper

import (
	"image"
	"os"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMonitorController_ApplyImage_StateConsistency(t *testing.T) {
	// Setup
	mockOS := new(MockOS)
	mockStore := new(MockImageStore)

	// Create invalid image (file missing)
	missingImg := provider.Image{
		ID:       "missing_id",
		FilePath: "/tmp/missing.jpg",
	}

	mc := &MonitorController{
		ID:      0,
		Monitor: Monitor{ID: 0, Rect: image.Rect(0, 0, 1920, 1080)},
		Store:   mockStore,
		os:      mockOS,
		State: &MonitorState{
			CurrentID:    "initial_id",
			CurrentImage: provider.Image{ID: "initial_id"},
		},
	}

	// Expect Stat to fail
	mockOS.On("Stat", "/tmp/missing.jpg").Return(nil, os.ErrNotExist)

	// Expect Store Update (clearing metadata)
	mockStore.On("Update", mock.Anything).Return(true)

	// Expect FetchRequest (triggered on failure)
	fetchRequested := false
	mc.OnFetchRequest = func() {
		fetchRequested = true
	}

	// Action
	mc.applyImage(missingImg)

	// Assert
	// The state should NOT have updated to "missing_id" because the file was missing
	// Current Bug: It DOES update.
	assert.NotEqual(t, "missing_id", mc.State.CurrentImage.ID, "State should not update if file is missing")
	assert.True(t, fetchRequested, "Should request fetch on failure")
}
