//go:build !linux

package wallpaper

import (
	"fmt"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGrowShuffle_PreservesPosition(t *testing.T) {
	mc := NewMonitorController(0, Monitor{ID: 0}, nil, nil, nil, nil, nil)

	// Start with 5 images, played through 3
	mc.State.ShuffleIDs = []string{"a", "b", "c", "d", "e"}
	mc.State.RandomPos = 3 // played: a, b, c; unplayed: d, e

	// Pool grows with 3 new images
	bucketIDs := []string{"a", "b", "c", "d", "e", "new1", "new2", "new3"}
	mc.growShuffle(bucketIDs)

	// Position must be preserved
	assert.Equal(t, 3, mc.State.RandomPos, "Position should not change after grow")
	// Total deck should have all 8 images
	assert.Equal(t, 8, len(mc.State.ShuffleIDs), "Deck should contain all images")
}

func TestGrowShuffle_PlayedImagesStayIntact(t *testing.T) {
	mc := NewMonitorController(0, Monitor{ID: 0}, nil, nil, nil, nil, nil)

	// Start with 5 images, played through 3
	mc.State.ShuffleIDs = []string{"a", "b", "c", "d", "e"}
	mc.State.RandomPos = 3

	bucketIDs := []string{"a", "b", "c", "d", "e", "new1", "new2"}
	mc.growShuffle(bucketIDs)

	// Played portion (first 3) must be unchanged
	assert.Equal(t, "a", mc.State.ShuffleIDs[0])
	assert.Equal(t, "b", mc.State.ShuffleIDs[1])
	assert.Equal(t, "c", mc.State.ShuffleIDs[2])
}

func TestGrowShuffle_NewIDsAppearInUnplayed(t *testing.T) {
	mc := NewMonitorController(0, Monitor{ID: 0}, nil, nil, nil, nil, nil)

	mc.State.ShuffleIDs = []string{"a", "b", "c"}
	mc.State.RandomPos = 2 // played: a, b; unplayed: c

	bucketIDs := []string{"a", "b", "c", "new1", "new2"}
	mc.growShuffle(bucketIDs)

	// All new IDs must be in the deck
	deckSet := make(map[string]bool)
	for _, id := range mc.State.ShuffleIDs {
		deckSet[id] = true
	}
	assert.True(t, deckSet["new1"], "new1 should be in deck")
	assert.True(t, deckSet["new2"], "new2 should be in deck")

	// New IDs must be in unplayed portion (index 2+)
	unplayed := mc.State.ShuffleIDs[mc.State.RandomPos:]
	unplayedSet := make(map[string]bool)
	for _, id := range unplayed {
		unplayedSet[id] = true
	}
	assert.True(t, unplayedSet["new1"], "new1 should be in unplayed portion")
	assert.True(t, unplayedSet["new2"], "new2 should be in unplayed portion")
}

func TestGrowShuffle_NoNewIDs_NoOp(t *testing.T) {
	mc := NewMonitorController(0, Monitor{ID: 0}, nil, nil, nil, nil, nil)

	mc.State.ShuffleIDs = []string{"a", "b", "c"}
	mc.State.RandomPos = 1

	// Bucket has same IDs — no growth
	bucketIDs := []string{"a", "b", "c"}
	mc.growShuffle(bucketIDs)

	assert.Equal(t, 3, len(mc.State.ShuffleIDs))
	assert.Equal(t, 1, mc.State.RandomPos)
}

func TestGrowShuffle_PoolShrinkTriggersFullRebuild(t *testing.T) {
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	mockIP := new(MockImageProcessor)
	cfg := GetConfig(NewMockPreferences())
	mc := NewMonitorController(0, Monitor{ID: 0}, mockStore, nil, mockOS, cfg, mockIP)

	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))
	resKey := "0x0"

	// Start with 5 images, played through 2
	mc.State.ShuffleIDs = []string{"a", "b", "c", "d", "e"}
	mc.State.RandomPos = 2

	// Pool shrinks to 3
	shrunkIDs := []string{"a", "b", "c"}
	mockStore.On("GetIDsForResolution", resKey).Return(shrunkIDs).Once()
	mockStore.On("GetByID", mock.Anything).Return(provider.Image{ID: "a", FilePath: "a.jpg"}, true).Once()
	mockStore.On("MarkSeen", "a.jpg").Return().Once()
	mockOS.On("Stat", "a.jpg").Return(nil, nil).Once()
	mockOS.On("SetWallpaper", "a.jpg", 0).Return(nil).Once()

	mc.next(true)

	// Full rebuild should reset position to 0
	assert.Equal(t, 3, len(mc.State.ShuffleIDs), "Deck should match shrunk pool")
	// Position should be 1 (after picking one image from rebuilt deck starting at 0)
	assert.Equal(t, 1, mc.State.RandomPos)
}

func TestGrowShuffle_ProviderInterleaving(t *testing.T) {
	// Simulate the real-world scenario: AIC images arrive first, then CMA
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	mockIP := new(MockImageProcessor)
	cfg := GetConfig(NewMockPreferences())
	mc := NewMonitorController(0, Monitor{ID: 0}, mockStore, nil, mockOS, cfg, mockIP)

	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))
	resKey := "0x0"

	// Phase 1: AIC batch arrives (10 images)
	aicIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		aicIDs[i] = fmt.Sprintf("AIC_%d", i)
	}
	mc.State.ShuffleIDs = aicIDs
	mc.State.RandomPos = 0

	// View 3 AIC images
	for i := 0; i < 3; i++ {
		mockStore.On("GetIDsForResolution", resKey).Return(aicIDs).Once()
		img := provider.Image{ID: aicIDs[i], FilePath: aicIDs[i] + ".jpg"}
		mockStore.On("GetByID", mock.Anything).Return(img, true).Once()
		mockStore.On("MarkSeen", mock.Anything).Return().Once()
		mockOS.On("Stat", mock.Anything).Return(nil, nil).Once()
		mockOS.On("SetWallpaper", mock.Anything, 0).Return(nil).Once()
		mc.next(true)
	}
	assert.Equal(t, 3, mc.State.RandomPos)

	// Phase 2: CMA batch arrives (10 more images)
	allIDs := make([]string, 20)
	copy(allIDs, aicIDs)
	for i := 0; i < 10; i++ {
		allIDs[10+i] = fmt.Sprintf("CMA_%d", i)
	}

	// Next call should trigger growShuffle (bucket 10→20)
	mockStore.On("GetIDsForResolution", resKey).Return(allIDs).Once()
	mockStore.On("GetByID", mock.Anything).Return(provider.Image{ID: "mixed", FilePath: "mixed.jpg"}, true).Once()
	mockStore.On("MarkSeen", mock.Anything).Return().Once()
	mockOS.On("Stat", mock.Anything).Return(nil, nil).Once()
	mockOS.On("SetWallpaper", mock.Anything, 0).Return(nil).Once()

	mc.next(true)

	// Position should be 4 (was 3, picked one more)
	assert.Equal(t, 4, mc.State.RandomPos)
	// Deck should have all 20 images
	assert.Equal(t, 20, len(mc.State.ShuffleIDs))

	// Played portion (first 3) should still be AIC (preserved)
	for i := 0; i < 3; i++ {
		assert.Contains(t, mc.State.ShuffleIDs[i], "AIC_",
			"Played portion should preserve original AIC images at index %d", i)
	}

	// Unplayed portion should contain BOTH AIC and CMA images (mixed)
	unplayed := mc.State.ShuffleIDs[3:]
	hasCMA := false
	hasAIC := false
	for _, id := range unplayed {
		if len(id) > 4 && id[:4] == "CMA_" {
			hasCMA = true
		}
		if len(id) > 4 && id[:4] == "AIC_" {
			hasAIC = true
		}
	}
	assert.True(t, hasCMA, "Unplayed portion should contain CMA images")
	assert.True(t, hasAIC, "Unplayed portion should contain remaining AIC images")
}
