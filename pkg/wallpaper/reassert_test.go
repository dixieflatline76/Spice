package wallpaper

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// reassertFixture wires a controller to a real on-disk image so applyImage's
// existence check passes and SetWallpaper is actually reached.
type reassertFixture struct {
	mc       *MonitorController
	mockOS   *MockOS
	store    *ImageStore
	imgPath  string
	imageID  string
	monitorX Monitor
}

func newReassertFixture(t *testing.T) *reassertFixture {
	t.Helper()

	dir := t.TempDir()
	imgPath := filepath.Join(dir, "img1.jpg")
	assert.NoError(t, os.WriteFile(imgPath, []byte("data"), 0644))

	store := NewImageStore()
	store.SetAsyncSave(false)
	store.Add(provider.Image{
		ID:              "img1",
		FilePath:        imgPath,
		DerivativePaths: map[string]string{"1920x1080": imgPath},
	})

	mockOS := &MockOS{}
	mockOS.On("Stat", mock.Anything).Return(nil, nil).Maybe()
	mockOS.On("SetWallpaper", mock.Anything, mock.Anything).Return(nil).Maybe()

	m := Monitor{ID: 0, Name: "Test", DevicePath: "Test", Rect: image.Rect(0, 0, 1920, 1080)}
	mc := NewMonitorController(0, m, store, nil, mockOS, nil, nil)

	return &reassertFixture{mc: mc, mockOS: mockOS, store: store, imgPath: imgPath, imageID: "img1", monitorX: m}
}

// TestReassert_RestoresPreviousWallpaper is the core fix. macOS forgets the
// wallpaper Spice set through NSWorkspace, so after a restart the app must set
// it again from its own persisted state rather than picking a new image.
func TestReassert_RestoresPreviousWallpaper(t *testing.T) {
	f := newReassertFixture(t)

	f.mc.RestoreState(PersistedMonitorState{CurrentID: "img1", AppliedPath: f.imgPath})
	f.mc.reassert()

	f.mockOS.AssertCalled(t, "SetWallpaper", f.imgPath, 0)
	assert.Equal(t, "img1", f.mc.State.CurrentImage.ID, "state should reflect the restored image")
}

// TestReassert_DoesNotAdvanceCursor distinguishes a re-assert from a normal
// change: the user must get the same image back, not the next one.
func TestReassert_DoesNotAdvanceCursor(t *testing.T) {
	f := newReassertFixture(t)

	f.mc.RestoreState(PersistedMonitorState{
		CurrentID:  "img1",
		RandomPos:  0,
		ShuffleIDs: []string{"img1"},
	})
	before := f.mc.State.RandomPos

	f.mc.reassert()

	assert.Equal(t, before, f.mc.State.RandomPos, "re-assert must not advance the shuffle cursor")
	assert.Equal(t, "img1", f.mc.State.CurrentID)
}

// TestReassert_FallsBackToAppliedPath covers the image leaving the store while
// its file is still on disk: better to keep the desktop than to blank it.
func TestReassert_FallsBackToAppliedPath(t *testing.T) {
	f := newReassertFixture(t)

	orphan := filepath.Join(t.TempDir(), "orphan.jpg")
	assert.NoError(t, os.WriteFile(orphan, []byte("data"), 0644))

	// CurrentID is dropped by RestoreState because it is not in the store,
	// leaving only the applied path to fall back on.
	f.mc.RestoreState(PersistedMonitorState{CurrentID: "vanished", AppliedPath: orphan})
	assert.Equal(t, "", f.mc.State.CurrentID)

	f.mc.reassert()

	f.mockOS.AssertCalled(t, "SetWallpaper", orphan, 0)
}

// TestReassert_NoStateFallsBackToNext covers the first launch after this
// feature ships: there is no state file, and the pre-Activate foreground sync
// has already spent its wallpaper request against an empty store. Without a
// fallback the desktop just stays on the macOS default.
func TestReassert_NoStateFallsBackToNext(t *testing.T) {
	f := newReassertFixture(t)

	f.mc.reassert()

	// The store holds one usable image for this monitor's resolution, so the
	// fallback rotation must put it on screen.
	f.mockOS.AssertCalled(t, "SetWallpaper", f.imgPath, 0)
}

// TestReassert_KeepsCurrentImageWhenAlreadyShowing verifies the fallback does
// not fire for a monitor that is already displaying something, which would
// churn the wallpaper for no reason.
func TestReassert_KeepsCurrentImageWhenAlreadyShowing(t *testing.T) {
	f := newReassertFixture(t)

	// Simulate a monitor mid-session: it has an image but no restorable state.
	f.mc.State.CurrentImage = provider.Image{ID: "already_showing"}

	f.mc.reassert()

	f.mockOS.AssertNotCalled(t, "SetWallpaper", mock.Anything, mock.Anything)
}

// TestReassert_MissingFileIsANoop verifies a purged image does not produce a
// spurious SetWallpaper call with a dead path.
func TestReassert_MissingFileIsANoop(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone.jpg")

	store := NewImageStore()
	store.SetAsyncSave(false)

	mockOS := &MockOS{}
	mockOS.On("Stat", missing).Return(nil, os.ErrNotExist).Maybe()
	mockOS.On("SetWallpaper", mock.Anything, mock.Anything).Return(nil).Maybe()

	m := Monitor{ID: 0, DevicePath: "Test", Rect: image.Rect(0, 0, 1920, 1080)}
	mc := NewMonitorController(0, m, store, nil, mockOS, nil, nil)

	mc.RestoreState(PersistedMonitorState{CurrentID: "gone", AppliedPath: missing})
	mc.reassert()

	mockOS.AssertNotCalled(t, "SetWallpaper", mock.Anything, mock.Anything)
}

// TestApplyImage_PersistsState verifies every wallpaper change is recorded, so
// there is something to restore after the next reboot.
func TestApplyImage_PersistsState(t *testing.T) {
	f := newReassertFixture(t)

	recorded := make(chan PersistedMonitorState, 1)
	f.mc.OnStatePersist = func(ps PersistedMonitorState) { recorded <- ps }

	img, ok := f.store.GetByID("img1")
	assert.True(t, ok)

	f.mc.State.CurrentID = "img1"
	f.mc.applyImage(img)

	// The callback is dispatched on a goroutine, so block until it lands.
	ps := <-recorded
	assert.Equal(t, "img1", ps.CurrentID)
	assert.Equal(t, f.imgPath, ps.AppliedPath)
	assert.Equal(t, "Test", ps.DevicePath)
	assert.Equal(t, "1920x1080", ps.ResKey)
}

// TestReassertRoundTrip_SurvivesRestart simulates the whole reboot path:
// session one applies a wallpaper and persists it, session two restores the
// state from disk and puts the same image back.
func TestReassertRoundTrip_SurvivesRestart(t *testing.T) {
	f := newReassertFixture(t)
	statePath := filepath.Join(t.TempDir(), monitorStateFileName)

	// --- Session one: apply and persist. ---
	stateStore := newMonitorStateStore(statePath)

	img, ok := f.store.GetByID("img1")
	assert.True(t, ok)
	f.mc.State.CurrentID = "img1"

	done := make(chan struct{})
	f.mc.OnStatePersist = func(ps PersistedMonitorState) {
		stateStore.Record(ps)
		close(done)
	}
	f.mc.applyImage(img)
	<-done
	stateStore.Flush()

	// --- Session two: fresh store and controller, same monitor. ---
	reloaded := newMonitorStateStore(statePath)
	reloaded.Load()

	ps, found := reloaded.Lookup("Test", 0)
	assert.True(t, found, "state must be readable in the next session")

	mockOS2 := &MockOS{}
	mockOS2.On("Stat", mock.Anything).Return(nil, nil).Maybe()
	mockOS2.On("SetWallpaper", mock.Anything, mock.Anything).Return(nil).Maybe()
	mc2 := NewMonitorController(0, f.monitorX, f.store, nil, mockOS2, nil, nil)

	mc2.RestoreState(ps)
	mc2.reassert()

	mockOS2.AssertCalled(t, "SetWallpaper", f.imgPath, 0)
}
