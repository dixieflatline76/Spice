package wallpaper

import (
	"context"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
)

func TestMonitorController_SyncState(t *testing.T) {
	mockStore := new(MockImageStore)

	imgID := "test_img"
	initialImg := provider.Image{ID: imgID, IsFavorited: false}
	updatedImg := provider.Image{ID: imgID, IsFavorited: true}

	mc := &MonitorController{
		ID:       0,
		Commands: make(chan Command, 10),
		Store:    mockStore,
		State: &MonitorState{
			CurrentID:    imgID,
			CurrentImage: initialImg,
		},
	}

	// Mock Store.GetByID to return the updated image
	mockStore.On("GetByID", imgID).Return(updatedImg, true)
	var updateCh <-chan struct{} = make(chan struct{})
	mockStore.On("GetUpdateChannel").Return(updateCh)

	// Run actor in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	// Action: Send SyncState command
	mc.Commands <- CmdSyncState

	// Wait for processing (actor is async)
	time.Sleep(100 * time.Millisecond)

	// Assert
	assert.True(t, mc.State.CurrentImage.IsFavorited, "Monitor state should have refreshed favorite status from store")
}
