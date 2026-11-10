package wallpaper

import (
	"os"
	"testing"
	"time"

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
	ResetConfig()
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

// shownImage returns the image recorded for monitorID in wp's claim registry.
func shownImage(wp *Plugin, monitorID int) (string, bool) {
	wp.shownMu.Lock()
	defer wp.shownMu.Unlock()
	id, ok := wp.shownImages[monitorID]
	return id, ok
}

func TestNext_DuplicateFallbackDoesNotReapplyOnStoreUpdate(t *testing.T) {
	wp := &Plugin{}
	ids := []string{"only"}

	mc0, _ := newDistinctTestController(t, 0, wp, ids)
	mc1, os1 := newDistinctTestController(t, 1, wp, ids)
	mc0.State.ShuffleIDs = []string{"only"}
	mc1.State.ShuffleIDs = []string{"only"}

	mc0.next(true)
	mc1.next(true)
	assert.True(t, mc1.State.DuplicateFallback)
	os1.AssertNumberOfCalls(t, "SetWallpaper", 1)

	// Store updates for images that don't fit this monitor still trigger next(false).
	// With no distinct image available, the current duplicate must stay put.
	for i := 0; i < 3; i++ {
		mc1.next(false)
	}

	assert.Equal(t, "only", mc1.State.CurrentID)
	assert.True(t, mc1.State.DuplicateFallback)
	assert.Equal(t, []string{"only"}, mc1.State.History, "no duplicate History entries")
	os1.AssertNumberOfCalls(t, "SetWallpaper", 1)
}

func TestNext_MissingFileRestoresClaim(t *testing.T) {
	wp := &Plugin{}
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, new(MockImageProcessor))
	mc.ClaimImage = wp.claimImage
	mc.State.CurrentImage = provider.Image{ID: "old", FilePath: "old.jpg"}
	mc.State.ShuffleIDs = []string{"gone"}
	wp.claimImage(1, "old", true)

	mockStore.On("GetIDsForResolution", "0x0").Return([]string{"gone"})
	mockStore.On("GetByID", "gone").Return(provider.Image{ID: "gone", FilePath: "gone.jpg"}, true)
	mockStore.On("ClearDerivatives", "gone").Return(true)
	mockOS.On("Stat", "gone.jpg").Return(nil, os.ErrNotExist)

	// next() claims "gone" before applyImage finds its file missing.
	mc.next(true)

	shown, _ := shownImage(wp, 1)
	assert.Equal(t, "old", shown, "a wallpaper that was never applied must not be recorded as shown")
	assert.True(t, wp.claimImage(0, "gone", false), "other monitors may still use the missing image once re-fetched")
	mockOS.AssertNotCalled(t, "SetWallpaper", mock.Anything, mock.Anything)
}

func TestNext_DuplicateWithMissingFileIsRetried(t *testing.T) {
	wp := &Plugin{}
	ids := []string{"a"}

	mc0, _ := newDistinctTestController(t, 0, wp, ids)
	mc0.State.ShuffleIDs = []string{"a"}
	mc0.next(true)

	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())
	mc1 := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, new(MockImageProcessor))
	mc1.ClaimImage = wp.claimImage
	mc1.State.ShuffleIDs = []string{"a"}

	mockStore.On("GetIDsForResolution", "0x0").Return(ids)
	mockStore.On("GetByID", "a").Return(provider.Image{ID: "a", FilePath: "a.jpg"}, true)
	mockStore.On("ClearDerivatives", "a").Return(true)
	mockStore.On("MarkSeen", mock.Anything).Return()
	mockOS.On("Stat", "a.jpg").Return(nil, os.ErrNotExist).Once()
	mockOS.On("Stat", "a.jpg").Return(nil, nil)
	mockOS.On("SetWallpaper", "a.jpg", 1).Return(nil)

	// The duplicate's file is missing, so nothing reaches the screen.
	mc1.next(true)
	assert.True(t, mc1.State.DuplicateFallback)
	mockOS.AssertNotCalled(t, "SetWallpaper", mock.Anything, mock.Anything)

	// Once the file is back, the next store update must apply it rather than
	// treat the never-shown duplicate as already on screen.
	mc1.next(false)
	mockOS.AssertCalled(t, "SetWallpaper", "a.jpg", 1)
}

func TestApplyImage_SetWallpaperFailureRestoresClaim(t *testing.T) {
	wp := &Plugin{}
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, new(MockImageProcessor))
	mc.ClaimImage = wp.claimImage
	mc.State.CurrentImage = provider.Image{ID: "old", FilePath: "old.jpg"}
	wp.claimImage(1, "old", true)

	mockOS.On("Stat", "new.jpg").Return(nil, nil)
	mockOS.On("SetWallpaper", "new.jpg", 1).Return(assert.AnError)
	mockStore.On("MarkSeen", mock.Anything).Return()

	mockOS.On("Stat", "newer.jpg").Return(nil, nil)
	mockOS.On("SetWallpaper", "newer.jpg", 1).Return(assert.AnError)

	mc.applyImage(provider.Image{ID: "new", FilePath: "new.jpg"})
	// A second failure in a row must still point back at what is on screen,
	// not at the image that failed first.
	mc.applyImage(provider.Image{ID: "newer", FilePath: "newer.jpg"})

	shown, _ := shownImage(wp, 1)
	assert.Equal(t, "old", shown, "a wallpaper the OS refused must not be recorded as shown")
	assert.Equal(t, "old", mc.State.CurrentImage.ID, "CurrentImage must stay on the image still on screen")
	assert.True(t, wp.claimImage(0, "new", false))
	assert.True(t, wp.claimImage(0, "newer", false))
}

func TestNext_FailedDuplicateIsRetried(t *testing.T) {
	wp := &Plugin{}
	ids := []string{"a"}

	mc0, _ := newDistinctTestController(t, 0, wp, ids)
	mc0.State.ShuffleIDs = []string{"a"}
	mc0.next(true)

	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())
	mc1 := NewMonitorController(1, Monitor{ID: 1}, mockStore, nil, mockOS, cfg, new(MockImageProcessor))
	mc1.ClaimImage = wp.claimImage
	mc1.State.ShuffleIDs = []string{"a"}

	mockStore.On("GetIDsForResolution", "0x0").Return(ids)
	mockStore.On("GetByID", "a").Return(provider.Image{ID: "a", FilePath: "a.jpg"}, true)
	mockStore.On("MarkSeen", mock.Anything).Return()
	mockOS.On("Stat", "a.jpg").Return(nil, nil)
	mockOS.On("SetWallpaper", "a.jpg", 1).Return(assert.AnError).Once()
	mockOS.On("SetWallpaper", "a.jpg", 1).Return(nil)

	// The OS refuses the duplicate, so nothing reaches the screen.
	mc1.next(true)
	assert.True(t, mc1.State.DuplicateFallback)
	mockOS.AssertNumberOfCalls(t, "SetWallpaper", 1)

	// The next store update must retry it rather than treat it as shown.
	mc1.next(false)
	mockOS.AssertNumberOfCalls(t, "SetWallpaper", 2)
}

func TestSyncMonitors_HotPluggedMonitorSkipsImageOnOtherMonitor(t *testing.T) {
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())

	ids := []string{"a", "b"}
	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil)).Maybe()
	mockStore.On("GetIDsForResolution", "0x0").Return(ids).Maybe()
	for _, imgID := range ids {
		mockStore.On("GetByID", imgID).Return(provider.Image{ID: imgID, FilePath: imgID + ".jpg"}, true).Maybe()
	}
	mockStore.On("MarkSeen", mock.Anything).Return().Maybe()
	mockOS.On("Stat", mock.Anything).Return(nil, nil).Maybe()

	applied := make(chan string, 4)
	mockOS.On("SetWallpaper", mock.Anything, 1).Run(func(args mock.Arguments) {
		applied <- args.String(0)
	}).Return(nil).Maybe()

	primary := Monitor{ID: 0, Name: "Primary"}
	mockOS.On("GetMonitors").Return([]Monitor{primary, {ID: 1, Name: "Secondary"}}, nil)

	wp := &Plugin{
		os:            mockOS,
		cfg:           cfg,
		store:         mockStore,
		Monitors:      make(map[int]*MonitorController),
		isDownloading: true, // keep RequestFetch a no-op
	}
	// Monitor 0 already exists (its actor isn't started) and is showing "a".
	wp.Monitors[0] = NewMonitorController(0, primary, mockStore, nil, mockOS, cfg, new(MockImageProcessor))
	wp.claimImage(0, "a", true)

	// Monitor 1 is plugged in while Spice is running.
	wp.SyncMonitors(false)
	t.Cleanup(func() {
		wp.monMu.Lock()
		defer wp.monMu.Unlock()
		if mc, ok := wp.Monitors[1]; ok {
			mc.Stop()
		}
	})

	select {
	case path := <-applied:
		assert.Equal(t, "b.jpg", path, "hot-plugged monitor must skip the image monitor 0 is showing")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the hot-plugged monitor to set a wallpaper")
	}
	shown, _ := shownImage(wp, 1)
	assert.Equal(t, "b", shown)
}

func TestSyncMonitors_RemovedMonitorReleasesImage(t *testing.T) {
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())

	primary := Monitor{ID: 0, Name: "Primary"}
	secondary := Monitor{ID: 1, Name: "Secondary"}
	mockOS.On("GetMonitors").Return([]Monitor{primary}, nil)

	wp := &Plugin{
		os:       mockOS,
		cfg:      cfg,
		Monitors: make(map[int]*MonitorController),
	}
	for _, m := range []Monitor{primary, secondary} {
		mc := NewMonitorController(m.ID, m, nil, nil, mockOS, cfg, nil)
		mc.ClaimImage = wp.claimImage
		wp.Monitors[m.ID] = mc
	}
	wp.claimImage(0, "a", true)
	wp.claimImage(1, "b", true)

	removed := wp.Monitors[1]

	// Monitor 1 is unplugged.
	wp.SyncMonitors(false)

	_, stillRecorded := shownImage(wp, 1)
	assert.False(t, stillRecorded, "removed monitor's image must be released")

	// Stop() doesn't wait for the actor, so a next() already queued on the
	// removed controller may still run. It must not record a claim again.
	mockStore := new(MockImageStore)
	mockStore.On("GetIDsForResolution", "0x0").Return([]string{"c"})
	mockStore.On("GetByID", "c").Return(provider.Image{ID: "c", FilePath: "c.jpg"}, true)
	mockStore.On("MarkSeen", mock.Anything).Return()
	mockOS.On("Stat", mock.Anything).Return(nil, nil)
	mockOS.On("SetWallpaper", mock.Anything, 1).Return(nil)
	removed.mu.Lock()
	removed.Store = mockStore
	removed.State.ShuffleIDs = []string{"c"}
	removed.next(true)
	removed.mu.Unlock()

	_, reclaimed := shownImage(wp, 1)
	assert.False(t, reclaimed, "a removed actor must not claim an image again")
	assert.True(t, wp.claimImage(0, "b", false), "other monitors may show the released image")
}
