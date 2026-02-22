package wallpaper

import (
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestToggleFavorite_AutoAdvance(t *testing.T) {
	// Setup
	wp := &Plugin{
		favoriter:          &mockDeepDeleteFavoriter{},
		store:              &MockImageStore{},
		manager:            &mockManager{},
		cfg:                GetConfig(NewMockPreferences()),
		providers:          make(map[string]provider.ImageProvider),
		Monitors:           make(map[int]*MonitorController),
		monitorMenu:        make(map[int]*MonitorMenuItems),
		fetchingInProgress: util.NewSafeBool(),
		runOnUI:            func(f func()) { f() },
	}

	mf := wp.favoriter.(*mockDeepDeleteFavoriter)
	ms := wp.store.(*MockImageStore)
	mm := wp.manager.(*mockManager)

	img := provider.Image{
		ID:          "fav_456",
		Provider:    "Favorites",
		IsFavorited: true,
	}

	// Mock deep delete path
	mf.On("RemoveFavorite", img).Return(nil)
	ms.On("Remove", img.ID).Return(img, true)
	mm.On("NotifyUser", mock.Anything, mock.Anything).Return()

	// Mock background telemetry
	ms.On("SeenCount").Return(0).Maybe()
	ms.On("Count").Return(0).Maybe()

	// Setup a monitor displaying this image
	mc := &MonitorController{
		ID:       0,
		Commands: make(chan Command, 10),
		State: &MonitorState{
			CurrentID:    img.ID,
			CurrentImage: provider.Image{ID: img.ID},
		},
	}
	wp.Monitors[0] = mc

	// Action
	wp.ToggleFavorite(img)

	// Wait briefly for async goroutine dispatch to complete
	time.Sleep(50 * time.Millisecond)

	// Verification
	mf.AssertExpectations(t)
	ms.AssertExpectations(t)

	// Drain all commands from the channel and verify CmdNext is among them.
	// Since the deadlock fix dispatches auto-advance via goroutine, CmdSyncState
	// may arrive before CmdNext on the buffered channel.
	var commands []Command
drainLoop:
	for {
		select {
		case cmd := <-mc.Commands:
			commands = append(commands, cmd)
		case <-time.After(200 * time.Millisecond):
			break drainLoop
		}
	}
	assert.Contains(t, commands, CmdNext, "Expected CmdNext to be dispatched among commands: %v", commands)
}
