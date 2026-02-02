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
	mc := NewMonitorController(1, Monitor{ID: 1}, nil, nil, nil, nil, nil) // Store nil for now
	assert.Equal(t, 1, mc.ID)
	assert.NotNil(t, mc.Commands)
	assert.NotNil(t, mc.State)
	assert.Equal(t, -1, mc.State.CurrentIndex)
}

func TestMonitorController_ProcessNext(t *testing.T) {
	// Plan:
	// 1. Setup Mock Store with 3 images
	// 2. Send CmdNext
	// 3. Verify State.CurrentIndex advances
	// 4. Verify output image path

	mockStore := new(MockImageStore)
	mockStore.On("Count").Return(3)
	mockStore.On("Get", 0).Return(provider.Image{FilePath: "img0.jpg"}, true)
	mockStore.On("Get", 1).Return(provider.Image{FilePath: "img1.jpg"}, true)
	mockStore.On("MarkSeen", "img0.jpg").Return()

	mockOS := new(MockOS)
	mockOS.On("SetWallpaper", "img0.jpg", 1).Return(nil)
	cfg := GetConfig(NewMockPreferences())
	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, mockIP)
	mc.State.RandomPos = 0                 // Start at 0
	mc.State.ShuffleOrder = []int{0, 1, 2} // Deterministic order

	// Run the loop in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	// Action: Next
	mc.Commands <- CmdNext

	// Wait for processing (Actor pattern)
	time.Sleep(50 * time.Millisecond)

	// Assert: Controller internal state should have advanced
	assert.Equal(t, 1, mc.State.RandomPos)
	assert.Equal(t, "img0.jpg", mc.State.CurrentImage.FilePath) // ShuffleOrder[0] is 0 -> img0
}

func TestMonitorController_ResolutionAwarePath(t *testing.T) {
	// Plan: Verify that MC selects DerivativePaths[WxH] over FilePath
	mockStore := new(MockImageStore)
	mockStore.On("Count").Return(1)

	img := provider.Image{
		ID:       "res_img",
		FilePath: "generic.jpg",
		DerivativePaths: map[string]string{
			"1920x1080": "hd.jpg",
			"3840x2160": "4k.jpg",
		},
	}
	mockStore.On("Get", 0).Return(img, true)
	mockStore.On("MarkSeen", "hd.jpg").Return()

	mockOS := new(MockOS)
	mockOS.On("SetWallpaper", "hd.jpg", 1).Return(nil)

	// Mock monitor with 1920x1080 resolution
	m := Monitor{
		ID:   1,
		Rect: image.Rect(0, 0, 1920, 1080),
	}

	cfg := GetConfig(NewMockPreferences())
	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(1, m, mockStore, nil, mockOS, cfg, mockIP)
	mc.State.ShuffleOrder = []int{0}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	mc.Commands <- CmdNext
	time.Sleep(50 * time.Millisecond)

	// Should have selected "hd.jpg"
	assert.Equal(t, "hd.jpg", mc.State.CurrentImage.FilePath) // CurrentImage.FilePath is set to generic but we expect the OS call used hd.jpg
	// Actually mc.applyImage sets mc.State.CurrentImage = img, and calls SetWallpaper(targetPath)
	// Let's verify our mock expectation was met
	mockOS.AssertExpectations(t)
}

func TestMonitorController_HistoryPrev(t *testing.T) {
	// Plan: Verify Prev walks back history
	mockStore := new(MockImageStore)
	mockStore.On("Get", 0).Return(provider.Image{FilePath: "img0.jpg"}, true)
	mockStore.On("MarkSeen", "img0.jpg").Return()

	mockOS := new(MockOS)
	mockOS.On("SetWallpaper", "img0.jpg", 1).Return(nil)

	cfg := GetConfig(NewMockPreferences())
	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, mockIP)
	mc.State.History = []int{0, 1}
	mc.State.CurrentIndex = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	mc.Commands <- CmdPrev
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, mc.State.CurrentIndex)
	assert.Equal(t, "img0.jpg", mc.State.CurrentImage.FilePath)
}

func TestMonitorController_Delete(t *testing.T) {
	// Plan: Send CmdDelete, verify Delete callback is invoked (mocked)
	// TODO: Needs DeleteDelegate interface
}
