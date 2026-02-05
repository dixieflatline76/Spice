package wallpaper

import (
	"context"
	"image"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
)

func TestMonitorController_Initialization(t *testing.T) {
	// Plan: Verify controller starts with correct ID and channel
	mc := NewMonitorController(1, Monitor{ID: 1}, nil, nil, nil, nil, nil)
	assert.Equal(t, 1, mc.ID)
	assert.NotNil(t, mc.Commands)
	assert.NotNil(t, mc.State)
	assert.Equal(t, "", mc.State.CurrentID)
}

func TestMonitorController_ProcessNext(t *testing.T) {
	mockStore := new(MockImageStore)

	// Expectations for "next"
	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))
	resKey := "0x0" // Monitor ID:1 has empty rect by default
	mockStore.On("GetIDsForResolution", resKey).Return([]string{"id0", "id1", "id2"})
	mockStore.On("GetByID", "id0").Return(provider.Image{ID: "id0", FilePath: "img0.jpg"}, true)
	mockStore.On("MarkSeen", "img0.jpg").Return()

	mockOS := new(MockOS)
	mockOS.On("Stat", "img0.jpg").Return(nil, nil)
	mockOS.On("SetWallpaper", "img0.jpg", 1).Return(nil)
	cfg := GetConfig(NewMockPreferences())
	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, mockIP)
	mc.State.RandomPos = 0
	mc.State.ShuffleIDs = []string{"id0", "id1", "id2"}

	// Run
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	mc.Commands <- CmdNext
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, mc.State.RandomPos)
	assert.Equal(t, "id0", mc.State.CurrentID)
	assert.Equal(t, "img0.jpg", mc.State.CurrentImage.FilePath)
}

func TestMonitorController_ResolutionAwarePath(t *testing.T) {
	mockStore := new(MockImageStore)

	img := provider.Image{
		ID:       "res_img",
		FilePath: "generic.jpg",
		DerivativePaths: map[string]string{
			"1920x1080": "hd.jpg",
			"3840x2160": "4k.jpg",
		},
	}

	resKey := "1920x1080"
	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))
	mockStore.On("GetIDsForResolution", resKey).Return([]string{"res_img"})
	mockStore.On("GetByID", "res_img").Return(img, true)
	mockStore.On("MarkSeen", "hd.jpg").Return()

	mockOS := new(MockOS)
	mockOS.On("Stat", "hd.jpg").Return(nil, nil)
	mockOS.On("SetWallpaper", "hd.jpg", 1).Return(nil)

	m := Monitor{
		ID:   1,
		Rect: image.Rect(0, 0, 1920, 1080),
	}

	cfg := GetConfig(NewMockPreferences())
	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(1, m, mockStore, nil, mockOS, cfg, mockIP)
	mc.State.ShuffleIDs = []string{"res_img"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	mc.Commands <- CmdNext
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, "hd.jpg", mc.State.CurrentImage.FilePath)
	mockOS.AssertExpectations(t)
}

func TestMonitorController_HistoryPrev(t *testing.T) {
	mockStore := new(MockImageStore)
	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))
	mockStore.On("GetByID", "id0").Return(provider.Image{ID: "id0", FilePath: "img0.jpg"}, true)
	mockStore.On("MarkSeen", "img0.jpg").Return()

	mockOS := new(MockOS)
	mockOS.On("Stat", "img0.jpg").Return(nil, nil)
	mockOS.On("SetWallpaper", "img0.jpg", 1).Return(nil)

	cfg := GetConfig(NewMockPreferences())
	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, mockIP)
	mc.State.History = []string{"id0", "id1"}
	mc.State.CurrentID = "id1"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	mc.Commands <- CmdPrev
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, "id0", mc.State.CurrentID)
	assert.Equal(t, "img0.jpg", mc.State.CurrentImage.FilePath)
}

func TestMonitorController_Delete(t *testing.T) {
	// Plan: Send CmdDelete, verify Delete callback is invoked (mocked)
	// TODO: Needs DeleteDelegate interface
}
