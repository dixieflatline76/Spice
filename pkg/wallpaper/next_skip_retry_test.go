package wallpaper

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNext_SkipsMissingFileAndAppliesNextValidImage(t *testing.T) {
	wp := &Plugin{}
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(0, Monitor{ID: 0}, mockStore, nil, mockOS, cfg, new(MockImageProcessor))
	mc.ClaimImage = wp.claimImage
	mc.State.CurrentID = "old"
	mc.State.CurrentImage = provider.Image{ID: "old", FilePath: "old.jpg"}
	mc.State.History = []string{"old"}
	mc.State.ShuffleIDs = []string{"missing1", "empty2", "valid"}
	mc.State.RandomPos = 0
	wp.claimImage(0, "old", true)

	ids := []string{"missing1", "empty2", "valid"}
	mockStore.On("GetIDsForResolution", "0x0").Return(ids)
	mockStore.On("GetByID", "missing1").Return(provider.Image{ID: "missing1", FilePath: "missing1.jpg"}, true)
	mockStore.On("GetByID", "empty2").Return(provider.Image{ID: "empty2", FilePath: ""}, true)
	mockStore.On("GetByID", "valid").Return(provider.Image{ID: "valid", FilePath: "valid.jpg"}, true)

	mockStore.On("ClearDerivatives", "missing1").Return(true)
	mockStore.On("MarkSeen", "valid.jpg").Return()

	mockOS.On("Stat", "missing1.jpg").Return(nil, os.ErrNotExist)
	mockOS.On("Stat", "valid.jpg").Return(nil, nil)
	mockOS.On("SetWallpaper", "valid.jpg", 0).Return(nil)

	fetchRequested := false
	mc.OnFetchRequest = func() {
		fetchRequested = true
	}

	// A single Next click must skip both broken entries and apply "valid".
	mc.next(true)

	assert.Equal(t, "valid", mc.State.CurrentID, "Next must advance past missing/empty files to the first valid image")
	assert.Equal(t, "valid", mc.State.CurrentImage.ID)
	assert.Equal(t, []string{"old", "valid"}, mc.State.History, "Unapplied images must not pollute History")
	assert.False(t, mc.State.WaitingForImages)
	assert.True(t, fetchRequested, "Missing file should still request a healing fetch")

	shown, ok := shownImage(wp, 0)
	require.True(t, ok)
	assert.Equal(t, "valid", shown)
	mockStore.AssertCalled(t, "ClearDerivatives", "missing1")
	mockOS.AssertCalled(t, "SetWallpaper", "valid.jpg", 0)
}

func TestNext_AllFilesMissingEntersWaitingForImagesWithoutPollutingState(t *testing.T) {
	wp := &Plugin{}
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(0, Monitor{ID: 0}, mockStore, nil, mockOS, cfg, new(MockImageProcessor))
	mc.ClaimImage = wp.claimImage
	mc.State.CurrentID = "old"
	mc.State.CurrentImage = provider.Image{ID: "old", FilePath: "old.jpg"}
	mc.State.History = []string{"old"}
	mc.State.ShuffleIDs = []string{"gone1", "gone2"}
	mc.State.RandomPos = 0
	wp.claimImage(0, "old", true)

	ids := []string{"gone1", "gone2"}
	mockStore.On("GetIDsForResolution", "0x0").Return(ids)
	mockStore.On("GetByID", "gone1").Return(provider.Image{ID: "gone1", FilePath: "gone1.jpg"}, true)
	mockStore.On("GetByID", "gone2").Return(provider.Image{ID: "gone2", FilePath: "gone2.jpg"}, true)
	mockStore.On("ClearDerivatives", "gone1").Return(true)
	mockStore.On("ClearDerivatives", "gone2").Return(true)
	mockOS.On("Stat", "gone1.jpg").Return(nil, os.ErrNotExist)
	mockOS.On("Stat", "gone2.jpg").Return(nil, os.ErrNotExist)

	mc.next(true)

	assert.True(t, mc.State.WaitingForImages, "When every candidate file is missing, monitor must wait for images")
	assert.True(t, mc.State.ManualRecovery)
	assert.Equal(t, "old", mc.State.CurrentID, "CurrentID must remain on the image still on screen")
	assert.Equal(t, "old", mc.State.CurrentImage.ID)
	assert.Equal(t, []string{"old"}, mc.State.History, "Missing files must not be appended to History")

	shown, _ := shownImage(wp, 0)
	assert.Equal(t, "old", shown)
	mockOS.AssertNotCalled(t, "SetWallpaper", mock.Anything, mock.Anything)
}

func TestNext_SkipsCurrentImageAfterReshuffle(t *testing.T) {
	wp := &Plugin{}
	ids := []string{"imgA", "imgB", "imgC"}
	mc, mockOS := newDistinctTestController(t, 0, wp, ids)

	// Monitor 0 is already showing imgA, and a reshuffle placed imgA at index 0.
	mc.State.CurrentID = "imgA"
	mc.State.CurrentImage = provider.Image{ID: "imgA", FilePath: "imgA.jpg"}
	mc.State.History = []string{"imgA"}
	mc.State.ShuffleIDs = []string{"imgA", "imgB", "imgC"}
	mc.State.RandomPos = 0
	wp.claimImage(0, "imgA", true)

	mc.next(true)

	assert.Equal(t, "imgB", mc.State.CurrentID, "Next must skip the image already on screen when other candidates exist")
	assert.Equal(t, "imgB", mc.State.CurrentImage.ID)
	assert.Equal(t, []string{"imgA", "imgB"}, mc.State.History)
	mockOS.AssertCalled(t, "SetWallpaper", "imgB.jpg", 0)
	mockOS.AssertNotCalled(t, "SetWallpaper", "imgA.jpg", 0)
}

func TestNext_SingleImagePoolAlreadyOnScreenDoesNotStarve(t *testing.T) {
	wp := &Plugin{}
	ids := []string{"only"}
	mc, mockOS := newDistinctTestController(t, 0, wp, ids)

	mc.State.CurrentID = "only"
	mc.State.CurrentImage = provider.Image{ID: "only", FilePath: "only.jpg"}
	mc.State.History = []string{"only"}
	mc.State.ShuffleIDs = []string{"only"}
	mc.State.RandomPos = 0
	wp.claimImage(0, "only", true)

	mc.next(true)

	assert.False(t, mc.State.WaitingForImages, "A single-image pool already on screen must not enter WaitingForImages")
	assert.Equal(t, "only", mc.State.CurrentID)
	assert.Equal(t, "only", mc.State.CurrentImage.ID)
	assert.Equal(t, []string{"only"}, mc.State.History, "Single-image fallback must not spam duplicate History entries")
	mockOS.AssertNotCalled(t, "SetWallpaper", mock.Anything, mock.Anything)

	shown, _ := shownImage(wp, 0)
	assert.Equal(t, "only", shown)
}

func TestNext_TwoMonitorsTwoImagesKeepsMonitorsDistinct(t *testing.T) {
	wp := &Plugin{}
	ids := []string{"imgA", "imgB"}

	mc0, os0 := newDistinctTestController(t, 0, wp, ids)
	mc1, _ := newDistinctTestController(t, 1, wp, ids)

	// Monitor 0 is showing imgA and Monitor 1 is showing imgB.
	mc0.State.ShuffleIDs = []string{"imgA", "imgB"}
	mc1.State.ShuffleIDs = []string{"imgA", "imgB"}
	mc0.next(true)
	mc1.next(true)
	require.Equal(t, "imgA", mc0.State.CurrentID)
	require.Equal(t, "imgB", mc1.State.CurrentID)
	os0.Calls = nil

	// Rotating Monitor 0 when the only other image is already on Monitor 1 must keep imgA
	// rather than collapsing both monitors onto imgB.
	mc0.next(true)

	assert.Equal(t, "imgA", mc0.State.CurrentID)
	assert.False(t, mc0.State.DuplicateFallback)
	os0.AssertNotCalled(t, "SetWallpaper", "imgB.jpg", 0)
}

func TestNext_CacheCapSwapPreservesPlayedAndAddsNewImages(t *testing.T) {
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(0, Monitor{ID: 0}, mockStore, nil, mockOS, cfg, new(MockImageProcessor))
	// Deck of 4 images where 2 were played ("evicted1", "played2") and 2 unplayed ("u1", "u2").
	// At cache cap, "evicted1" was evicted and "new1" arrived -> bucket length is still 4!
	mc.State.ShuffleIDs = []string{"evicted1", "played2", "u1", "u2"}
	mc.State.RandomPos = 2
	mc.State.CurrentID = "played2"
	mc.State.CurrentImage = provider.Image{ID: "played2", FilePath: "played2.jpg"}
	mc.pendingUpdate = true

	swappedBucket := []string{"played2", "u1", "u2", "new1"}
	mockStore.On("GetIDsForResolution", "0x0").Return(swappedBucket)
	for _, id := range swappedBucket {
		mockStore.On("GetByID", id).Return(provider.Image{ID: id, FilePath: id + ".jpg"}, true).Maybe()
	}
	mockStore.On("MarkSeen", mock.Anything).Return()
	mockOS.On("Stat", mock.Anything).Return(nil, nil)
	mockOS.On("SetWallpaper", mock.Anything, 0).Return(nil)

	mc.next(true)

	// "played2" was already played (index 0 after pruning "evicted1") and must stay in played prefix,
	// while "new1" must now be in the deck and "evicted1" must be gone.
	assert.Equal(t, "played2", mc.State.ShuffleIDs[0], "Remaining played image must stay in played prefix")
	assert.Contains(t, mc.State.ShuffleIDs, "new1", "Newly arrived image at cache cap must enter shuffle deck")
	assert.NotContains(t, mc.State.ShuffleIDs, "evicted1", "Evicted image must be pruned from shuffle deck")
	assert.NotEqual(t, "played2", mc.State.CurrentID, "Next must pick from the unplayed portion, not re-play played2")
}

func TestFetchNewImages_DoesNotBroadcastDestructiveFullShuffle(t *testing.T) {
	mockStore := new(MockImageStore)
	mockStore.On("GetByID", mock.Anything).Return(provider.Image{}, false)

	mockOS := new(MockOS)
	mockOS.On("GetMonitors").Return([]Monitor{}, nil).Maybe()

	mockManager := new(MockPluginManager)
	mockManager.On("NotifyUser", mock.Anything, mock.Anything).Maybe()

	mockPipeline := new(MockPipeline)
	mockPipeline.On("Submit", mock.Anything, mock.Anything).Return(true)

	prov := new(MockImageProvider)
	prov.On("ID").Return("MetMuseum").Maybe()
	prov.On("Type").Return(provider.TypeMuseum).Maybe()

	var wg sync.WaitGroup
	wg.Add(1)
	prov.On("FetchImages", mock.Anything, "highlights", 1).Return([]provider.Image{{ID: "1"}}, nil).Run(func(args mock.Arguments) {
		wg.Done()
	}).Once()

	workingDir := config.GetWorkingDir()
	_ = os.MkdirAll(filepath.Join(workingDir, "wallpaper_downloads"), 0755)

	mc := NewMonitorController(0, Monitor{ID: 0}, mockStore, nil, mockOS, nil, nil)
	mc.State.ShuffleIDs = []string{"a", "b", "c", "d"}
	mc.State.RandomPos = 3

	wp := &Plugin{
		providers:          map[string]provider.ImageProvider{"MetMuseum": prov},
		queryPages:         make(map[string]*util.SafeCounter),
		fetchingInProgress: util.NewSafeBool(),
		store:              mockStore,
		ctx:                context.Background(),
		queryContexts:      make(map[string]context.Context),
		queryCancelFuncs:   make(map[string]context.CancelFunc),
		manager:            mockManager,
		Monitors:           map[int]*MonitorController{0: mc},
		os:                 mockOS,
		jobSubmitter:       mockPipeline,
		cfg: &Config{
			Queries: []ImageQuery{
				{ID: "q1", Provider: "MetMuseum", Active: true, URL: "highlights", Description: "Highlights"},
			},
		},
	}

	wp.FetchNewImages(true)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		// Give FetchNewImages post-WaitGroup block time to complete
		time.Sleep(50 * time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for FetchNewImages")
	}

	// Background fetch must NOT push CmdUpdateShuffle onto mc.Commands (which would wipe RandomPos=0 via rebuildShuffle).
	// Instead, next() reconciles new arrivals incrementally via growShuffle/reconcileShuffle.
	assert.Empty(t, mc.Commands, "Background fetch must not dispatch CmdUpdateShuffle that resets RandomPos to 0")
}
