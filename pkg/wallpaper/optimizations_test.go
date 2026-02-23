//go:build !linux

package wallpaper

import (
	"fmt"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMonitorController_HistoryCapping(t *testing.T) {
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	mockIP := new(MockImageProcessor)
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, mockIP)

	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))
	resKey := "0x0"

	// Setup a large bucket of IDs
	ids := make([]string, 150)
	for i := 0; i < 150; i++ {
		ids[i] = fmt.Sprintf("id%d", i)
	}

	// Force initial shuffle
	mc.State.ShuffleIDs = ids
	mc.State.RandomPos = 0

	// Simulate many "next" calls
	for i := 0; i < 150; i++ {
		mockStore.On("GetIDsForResolution", resKey).Return(ids).Once()
		mockStore.On("GetByID", mock.Anything).Return(provider.Image{ID: "dummy", FilePath: "dummy.jpg"}, true).Once()
		mockStore.On("MarkSeen", "dummy.jpg").Return().Once()
		mockOS.On("Stat", "dummy.jpg").Return(nil, nil).Once()
		mockOS.On("SetWallpaper", "dummy.jpg", 1).Return(nil).Once()

		mc.next(true)
	}

	// Verify history size is capped at 100
	assert.Equal(t, 100, len(mc.State.History))
}

func TestMonitorController_ReshuffleOptimization(t *testing.T) {
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	mockIP := new(MockImageProcessor)
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, mockIP)
	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))
	resKey := "0x0"

	// 1. Initial next call
	initialIDs := []string{"id1", "id2"}
	mc.State.ShuffleIDs = initialIDs
	mc.State.RandomPos = 0

	mockStore.On("GetIDsForResolution", resKey).Return(initialIDs).Once()
	mockStore.On("GetByID", mock.Anything).Return(provider.Image{ID: "id1", FilePath: "img1.jpg"}, true).Once()
	mockStore.On("MarkSeen", "img1.jpg").Return().Once()
	mockOS.On("Stat", "img1.jpg").Return(nil, nil).Once()
	mockOS.On("SetWallpaper", "img1.jpg", 1).Return(nil).Once()

	mc.next(true)
	assert.Equal(t, 1, mc.State.RandomPos)

	// 2. Set pendingUpdate = true
	mc.pendingUpdate = true

	// 3. Call next again. Should NOT reshuffle.
	mockStore.On("GetIDsForResolution", resKey).Return(initialIDs).Once()
	mockStore.On("GetByID", mock.Anything).Return(provider.Image{ID: "id2", FilePath: "img2.jpg"}, true).Once()
	mockStore.On("MarkSeen", "img2.jpg").Return().Once()
	mockOS.On("Stat", "img2.jpg").Return(nil, nil).Once()
	mockOS.On("SetWallpaper", "img2.jpg", 1).Return(nil).Once()

	mc.next(true)
	assert.Equal(t, 0, mc.State.RandomPos)
	assert.False(t, mc.pendingUpdate)

	// 4. Manual update shuffle
	mockStore.On("GetIDsForResolution", resKey).Return(initialIDs).Once()
	mc.updateShuffle()
	assert.Equal(t, 0, mc.State.RandomPos)
}
