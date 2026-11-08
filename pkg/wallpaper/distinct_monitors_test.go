package wallpaper

import (
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// newDistinctTestController builds a controller wired to a shared Plugin claim
// registry, with a store that serves the given IDs for its resolution bucket.
func newDistinctTestController(t *testing.T, id int, wp *Plugin, ids []string) (*MonitorController, *MockOS) {
	t.Helper()
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	mockIP := new(MockImageProcessor)
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(id, Monitor{ID: id}, mockStore, nil, mockOS, cfg, mockIP)
	mc.ClaimImage = wp.claimImage

	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil)).Maybe()
	mockStore.On("GetIDsForResolution", "0x0").Return(ids).Maybe()
	for _, imgID := range ids {
		mockStore.On("GetByID", imgID).Return(provider.Image{ID: imgID, FilePath: imgID + ".jpg"}, true).Maybe()
	}
	mockStore.On("MarkSeen", mock.Anything).Return().Maybe()
	mockOS.On("Stat", mock.Anything).Return(nil, nil).Maybe()
	mockOS.On("SetWallpaper", mock.Anything, id).Return(nil).Maybe()

	return mc, mockOS
}

func TestNext_SkipsImageShownOnOtherMonitor(t *testing.T) {
	wp := &Plugin{}
	ids := []string{"a", "b", "c"}

	mc0, os0 := newDistinctTestController(t, 0, wp, ids)
	mc1, os1 := newDistinctTestController(t, 1, wp, ids)

	// Both decks are in the same order: the pre-fix behaviour would put "a" on both.
	mc0.State.ShuffleIDs = []string{"a", "b", "c"}
	mc1.State.ShuffleIDs = []string{"a", "b", "c"}

	mc0.next(true)
	mc1.next(true)

	assert.Equal(t, "a", mc0.State.CurrentID)
	assert.Equal(t, "b", mc1.State.CurrentID, "monitor 1 must skip the image monitor 0 is showing")
	assert.False(t, mc1.State.DuplicateFallback)
	os0.AssertCalled(t, "SetWallpaper", "a.jpg", 0)
	os1.AssertCalled(t, "SetWallpaper", "b.jpg", 1)
	os1.AssertNotCalled(t, "SetWallpaper", "a.jpg", 1)
}

func TestNext_FallsBackToDuplicateWhenOnlyOneImage(t *testing.T) {
	wp := &Plugin{}
	ids := []string{"only"}

	mc0, _ := newDistinctTestController(t, 0, wp, ids)
	mc1, os1 := newDistinctTestController(t, 1, wp, ids)
	mc0.State.ShuffleIDs = []string{"only"}
	mc1.State.ShuffleIDs = []string{"only"}

	mc0.next(true)
	mc1.next(true)

	assert.Equal(t, "only", mc1.State.CurrentID, "a duplicate beats an empty screen")
	assert.True(t, mc1.State.DuplicateFallback, "monitor should retry when more images arrive")
	assert.False(t, mc0.State.DuplicateFallback)
	os1.AssertCalled(t, "SetWallpaper", "only.jpg", 1)
}

func TestNext_DuplicateResolvesOnceStoreGrows(t *testing.T) {
	wp := &Plugin{}

	mc0, _ := newDistinctTestController(t, 0, wp, []string{"a"})
	mc0.State.ShuffleIDs = []string{"a"}
	mc0.next(true)

	// Monitor 1 starts with the single-image pool, then the pool grows.
	mc1, _ := newDistinctTestController(t, 1, wp, []string{"a"})
	mc1.State.ShuffleIDs = []string{"a"}
	mc1.next(true)
	assert.True(t, mc1.State.DuplicateFallback)

	grown, os1 := newDistinctTestController(t, 1, wp, []string{"a", "b"})
	grown.State = mc1.State
	grown.State.ShuffleIDs = []string{"a", "b"}
	grown.State.RandomPos = 0
	grown.next(false)

	assert.Equal(t, "b", grown.State.CurrentID)
	assert.False(t, grown.State.DuplicateFallback)
	os1.AssertCalled(t, "SetWallpaper", "b.jpg", 1)
}

func TestClaimImage_ForceAndRelease(t *testing.T) {
	wp := &Plugin{}

	assert.True(t, wp.claimImage(0, "x", false))
	assert.False(t, wp.claimImage(1, "x", false), "x is already on monitor 0")
	assert.True(t, wp.claimImage(0, "x", false), "a monitor may re-claim its own image")
	assert.True(t, wp.claimImage(1, "x", true), "force allows a duplicate")

	// Monitor 0 moving on frees x for nobody else, but y is now taken.
	assert.True(t, wp.claimImage(0, "y", false))
	assert.False(t, wp.claimImage(1, "y", false))
}
