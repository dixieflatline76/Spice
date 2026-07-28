package wallpaper

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
)

func newTestStateStore(t *testing.T) *monitorStateStore {
	t.Helper()
	s := newMonitorStateStore(filepath.Join(t.TempDir(), monitorStateFileName))
	// Write synchronously so tests never race the debounce timer.
	s.debounce = time.Nanosecond
	return s
}

func TestMonitorStateStore_RoundTrip(t *testing.T) {
	store := newTestStateStore(t)

	store.Record(PersistedMonitorState{
		DevicePath:  "Built-in Retina Display",
		Index:       0,
		CurrentID:   "Wikimedia_123",
		AppliedPath: "/data/wallpaper_downloads/fitted/x/Wikimedia_123.jpg",
		ResKey:      "3456x2234",
		RandomPos:   4,
		ShuffleIDs:  []string{"a", "b"},
		History:     []string{"h1"},
	})
	store.Flush()

	reloaded := newMonitorStateStore(store.path)
	reloaded.Load()

	ps, ok := reloaded.Lookup("Built-in Retina Display", 0)
	assert.True(t, ok)
	assert.Equal(t, "Wikimedia_123", ps.CurrentID)
	assert.Equal(t, "/data/wallpaper_downloads/fitted/x/Wikimedia_123.jpg", ps.AppliedPath)
	assert.Equal(t, "3456x2234", ps.ResKey)
	assert.Equal(t, 4, ps.RandomPos)
	assert.Equal(t, []string{"a", "b"}, ps.ShuffleIDs)
	assert.Equal(t, []string{"h1"}, ps.History)
}

func TestMonitorStateStore_MissingFileIsNotAnError(t *testing.T) {
	store := newMonitorStateStore(filepath.Join(t.TempDir(), "does-not-exist.json"))
	store.Load()

	_, ok := store.Lookup("anything", 0)
	assert.False(t, ok)
	assert.Empty(t, store.CurrentIDs())
}

// TestMonitorStateStore_CorruptFileIsDiscarded verifies a truncated or garbled
// file cannot take down startup, and does not wipe anything on its own.
func TestMonitorStateStore_CorruptFileIsDiscarded(t *testing.T) {
	path := filepath.Join(t.TempDir(), monitorStateFileName)
	assert.NoError(t, os.WriteFile(path, []byte(`{"version":1,"monitors":{"a":`), 0600))

	store := newMonitorStateStore(path)
	assert.NotPanics(t, func() { store.Load() })

	_, ok := store.Lookup("a", 0)
	assert.False(t, ok)
}

func TestMonitorStateStore_UnknownVersionIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), monitorStateFileName)
	blob, err := json.Marshal(monitorStateFile{
		Version:  monitorStateVersion + 99,
		Monitors: map[string]PersistedMonitorState{"x#0": {DevicePath: "x", CurrentID: "img"}},
	})
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(path, blob, 0600))

	store := newMonitorStateStore(path)
	store.Load()

	_, ok := store.Lookup("x", 0)
	assert.False(t, ok, "a future format must not be interpreted as the current one")
}

// TestMonitorStateStore_LookupFallsBackToDevicePath covers the display whose
// index shifted, e.g. after unplugging a second monitor.
func TestMonitorStateStore_LookupFallsBackToDevicePath(t *testing.T) {
	store := newTestStateStore(t)
	store.Record(PersistedMonitorState{DevicePath: "MSI MP273A", Index: 2, CurrentID: "img2"})

	ps, ok := store.Lookup("MSI MP273A", 0)
	assert.True(t, ok, "should fall back to a device-path match")
	assert.Equal(t, "img2", ps.CurrentID)
}

// TestMonitorStateStore_ExactKeyWinsOverFallback ensures twin displays do not
// steal each other's wallpaper when both keys are present.
func TestMonitorStateStore_ExactKeyWinsOverFallback(t *testing.T) {
	store := newTestStateStore(t)
	store.Record(PersistedMonitorState{DevicePath: "Twin", Index: 0, CurrentID: "img0"})
	store.Record(PersistedMonitorState{DevicePath: "Twin", Index: 1, CurrentID: "img1"})

	ps, ok := store.Lookup("Twin", 1)
	assert.True(t, ok)
	assert.Equal(t, "img1", ps.CurrentID)
}

func TestMonitorStateStore_CurrentIDs(t *testing.T) {
	store := newTestStateStore(t)
	store.Record(PersistedMonitorState{DevicePath: "A", Index: 0, CurrentID: "img_a"})
	store.Record(PersistedMonitorState{DevicePath: "B", Index: 1, CurrentID: "img_b"})
	store.Record(PersistedMonitorState{DevicePath: "C", Index: 2, CurrentID: ""})

	ids := store.CurrentIDs()
	assert.True(t, ids["img_a"])
	assert.True(t, ids["img_b"])
	assert.Len(t, ids, 2, "empty IDs must not be recorded as protected")
}

// TestMonitorStateStore_NilReceiver documents the tolerance a Plugin built
// without Init relies on.
func TestMonitorStateStore_NilReceiver(t *testing.T) {
	var store *monitorStateStore
	assert.NotPanics(t, func() {
		store.Load()
		store.Record(PersistedMonitorState{DevicePath: "A"})
		store.Flush()
		_, ok := store.Lookup("A", 0)
		assert.False(t, ok)
		assert.Nil(t, store.CurrentIDs())
	})
}

// newRestoreController builds a controller wired to a real store holding the
// given image IDs.
func newRestoreController(t *testing.T, ids ...string) *MonitorController {
	t.Helper()

	store := NewImageStore()
	store.SetAsyncSave(false)
	for _, id := range ids {
		store.Add(provider.Image{ID: id, FilePath: "/tmp/" + id + ".jpg"})
	}

	m := Monitor{ID: 0, Name: "Test", DevicePath: "Test", Rect: image.Rect(0, 0, 1920, 1080)}
	return NewMonitorController(0, m, store, nil, &MockOS{}, nil, nil)
}

func TestRestoreState_SeedsFromPersistedState(t *testing.T) {
	mc := newRestoreController(t, "img1", "img2", "img3")

	mc.RestoreState(PersistedMonitorState{
		CurrentID:   "img1",
		AppliedPath: "/tmp/img1.jpg",
		RandomPos:   1,
		ShuffleIDs:  []string{"img2", "img3"},
		History:     []string{"img2"},
	})

	assert.Equal(t, "img1", mc.State.CurrentID)
	assert.Equal(t, "/tmp/img1.jpg", mc.restoredPath)
	assert.Equal(t, 1, mc.State.RandomPos)
	assert.Equal(t, []string{"img2", "img3"}, mc.State.ShuffleIDs)
	assert.Equal(t, []string{"img2"}, mc.State.History)
}

// TestRestoreState_DropsIDsMissingFromStore prevents a stale state file from
// resurrecting images the user deleted between runs.
func TestRestoreState_DropsIDsMissingFromStore(t *testing.T) {
	mc := newRestoreController(t, "kept")

	mc.RestoreState(PersistedMonitorState{
		CurrentID:  "deleted",
		ShuffleIDs: []string{"kept", "deleted"},
		History:    []string{"deleted"},
	})

	assert.Equal(t, "", mc.State.CurrentID, "an image no longer in the store must not be restored")
	assert.Equal(t, []string{"kept"}, mc.State.ShuffleIDs)
	assert.Empty(t, mc.State.History)
}

// TestRestoreState_ClampsRandomPos guards the cursor after entries are dropped.
func TestRestoreState_ClampsRandomPos(t *testing.T) {
	mc := newRestoreController(t, "kept")

	mc.RestoreState(PersistedMonitorState{
		RandomPos:  9,
		ShuffleIDs: []string{"kept", "gone", "gone2"},
	})

	assert.Len(t, mc.State.ShuffleIDs, 1)
	assert.Equal(t, 0, mc.State.RandomPos, "an out-of-range cursor must be reset")
}

// TestRestoreState_KeepsAppliedPathWhenImageIsGone verifies the fallback data
// survives, so reassert can still put the file back on screen.
func TestRestoreState_KeepsAppliedPathWhenImageIsGone(t *testing.T) {
	mc := newRestoreController(t)

	mc.RestoreState(PersistedMonitorState{CurrentID: "gone", AppliedPath: "/tmp/gone.jpg"})

	assert.Equal(t, "", mc.State.CurrentID)
	assert.Equal(t, "/tmp/gone.jpg", mc.restoredPath)
}

// TestSnapshotStateLocked captures what gets written after an apply.
func TestSnapshotStateLocked(t *testing.T) {
	mc := newRestoreController(t, "img1")
	mc.State.CurrentID = "img1"
	mc.State.RandomPos = 2
	mc.State.ShuffleIDs = []string{"img1"}

	ps := mc.snapshotStateLocked("/data/img1.jpg")

	assert.Equal(t, "Test", ps.DevicePath)
	assert.Equal(t, 0, ps.Index)
	assert.Equal(t, "img1", ps.CurrentID)
	assert.Equal(t, "/data/img1.jpg", ps.AppliedPath)
	assert.Equal(t, "1920x1080", ps.ResKey)
	assert.Equal(t, 2, ps.RandomPos)
	assert.False(t, ps.UpdatedAt.IsZero())

	// The snapshot must not alias live state, which the actor keeps mutating.
	mc.State.ShuffleIDs[0] = "mutated"
	assert.Equal(t, []string{"img1"}, ps.ShuffleIDs)
}

func TestMonitorStateKey(t *testing.T) {
	assert.Equal(t, "Built-in#0", monitorStateKey("Built-in", 0))
	assert.Equal(t, "Built-in#2", monitorStateKey("Built-in", 2))
	assert.NotEqual(t, monitorStateKey("Twin", 0), monitorStateKey("Twin", 1))
}
