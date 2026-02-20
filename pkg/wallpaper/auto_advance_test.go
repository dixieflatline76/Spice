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
			CurrentImage: provider.Image{ID: img.ID},
		},
	}
	wp.Monitors[0] = mc

	// Action
	wp.ToggleFavorite(img)

	// Verification
	mf.AssertExpectations(t)
	ms.AssertExpectations(t)

	// Check if CmdNext was dispatched to the monitor
	select {
	case cmd := <-mc.Commands:
		assert.Equal(t, CmdNext, cmd, "Expected CmdNext to be dispatched")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timed out waiting for CmdNext dispatch")
	}
}
