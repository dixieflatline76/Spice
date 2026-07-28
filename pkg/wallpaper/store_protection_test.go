package wallpaper

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
)

// newProtectionStore builds a store backed by a temp dir, plus a helper that
// registers an image with a real master file on disk so masterFileExists
// passes and Sync does not delete it for unrelated reasons.
func newProtectionStore(t *testing.T) (*ImageStore, func(id, queryID string) provider.Image) {
	t.Helper()

	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	addImage := func(id, queryID string) provider.Image {
		path := filepath.Join(tmpDir, id+".jpg")
		assert.NoError(t, os.WriteFile(path, []byte("data"), 0644))
		img := provider.Image{
			ID:              id,
			SourceQueryID:   queryID,
			FilePath:        path,
			DerivativePaths: map[string]string{"1920x1080": id + "_d.jpg"},
		}
		store.Add(img)
		return img
	}

	return store, addImage
}

// TestSyncPrune_KeepsProtectedImage is the core regression: the image on screen
// is the oldest candidate, so the old prune (finalImages[:excess]) deleted it
// and left the desktop pointing at a file that no longer exists.
func TestSyncPrune_KeepsProtectedImage(t *testing.T) {
	store, addImage := newProtectionStore(t)

	for i := 0; i < 10; i++ {
		addImage(fmt.Sprintf("img%02d", i), "q1")
	}

	// img00 is the oldest entry and therefore first in line to be pruned.
	store.SetProtectedIDsFunc(func() map[string]bool {
		return map[string]bool{"img00": true}
	})

	store.Sync(1, nil, map[string]bool{"q1": true})

	known := store.GetKnownIDs()
	assert.True(t, known["img00"], "the on-screen image must survive pruning")
	assert.Equal(t, 1, store.Count(), "prune must still reach the configured limit")

	// The protected master file must remain on disk for the next re-assert.
	_, err := os.Stat(filepath.Join(store.fm.GetDownloadDir(), "img00.jpg"))
	assert.NoError(t, err, "protected master file was deleted")
}

// TestSyncPrune_ProtectedDoesNotBlockOthers verifies the prune still removes the
// requested number of images by skipping past the protected ones.
func TestSyncPrune_ProtectedDoesNotBlockOthers(t *testing.T) {
	store, addImage := newProtectionStore(t)

	for i := 0; i < 5; i++ {
		addImage(fmt.Sprintf("img%02d", i), "q1")
	}

	store.SetProtectedIDsFunc(func() map[string]bool {
		return map[string]bool{"img00": true, "img01": true}
	})

	// Limit 3 on 5 images means 2 must go; both oldest are protected, so the
	// prune has to walk on to img02 and img03.
	store.Sync(3, nil, map[string]bool{"q1": true})

	known := store.GetKnownIDs()
	assert.Equal(t, 3, store.Count())
	assert.True(t, known["img00"])
	assert.True(t, known["img01"])
	assert.False(t, known["img02"], "unprotected image should have been pruned")
	assert.False(t, known["img03"], "unprotected image should have been pruned")
	assert.True(t, known["img04"])
}

// TestSyncPrune_AllProtectedKeepsEverything covers the degenerate case where
// every candidate is on screen. Keeping them all is correct: overshooting the
// cache limit is strictly better than blanking a display.
func TestSyncPrune_AllProtectedKeepsEverything(t *testing.T) {
	store, addImage := newProtectionStore(t)

	protected := map[string]bool{}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("img%02d", i)
		addImage(id, "q1")
		protected[id] = true
	}

	store.SetProtectedIDsFunc(func() map[string]bool { return protected })

	store.Sync(1, nil, map[string]bool{"q1": true})

	assert.Equal(t, 3, store.Count(), "no protected image may be pruned")
}

// TestSyncAction_ProtectedSurvivesInactiveQuery verifies the protected check
// short-circuits strict mode. Deactivating a query while its image is on screen
// used to delete the master out from under the live desktop.
func TestSyncAction_ProtectedSurvivesInactiveQuery(t *testing.T) {
	store, addImage := newProtectionStore(t)

	addImage("onscreen", "gone")
	addImage("other", "gone")

	store.SetProtectedIDsFunc(func() map[string]bool {
		return map[string]bool{"onscreen": true}
	})

	// activeQueryIDs no longer contains "gone", so strict mode wants both gone.
	store.Sync(100, nil, map[string]bool{"q1": true})

	known := store.GetKnownIDs()
	assert.True(t, known["onscreen"], "protected image must survive an inactive query")
	assert.False(t, known["other"], "unprotected image from an inactive query must be deleted")
}

// TestSyncAction_ProtectedIsNotInvalidated verifies a flag mismatch does not
// wipe the derivative the desktop is currently rendering.
func TestSyncAction_ProtectedIsNotInvalidated(t *testing.T) {
	store, addImage := newProtectionStore(t)

	img := addImage("onscreen", "q1")
	assert.NotEmpty(t, img.DerivativePaths)

	store.SetProtectedIDsFunc(func() map[string]bool {
		return map[string]bool{"onscreen": true}
	})

	// A target flag the stored image does not carry normally triggers
	// ImageActionInvalidate, which clears DerivativePaths.
	store.Sync(100, map[string]bool{"smart_fit": true}, map[string]bool{"q1": true})

	list := store.List()
	assert.Len(t, list, 1)
	assert.NotEmpty(t, list[0].DerivativePaths, "protected image must keep its derivative paths")
}

// TestSyncPrune_NilProtectedFuncIsLegacyBehavior guards against a regression in
// the default path: with no callback set, Sync must behave exactly as before.
func TestSyncPrune_NilProtectedFuncIsLegacyBehavior(t *testing.T) {
	store, addImage := newProtectionStore(t)

	for i := 0; i < 5; i++ {
		addImage(fmt.Sprintf("img%02d", i), "q1")
	}

	store.Sync(2, nil, map[string]bool{"q1": true})

	known := store.GetKnownIDs()
	assert.Equal(t, 2, store.Count())
	assert.False(t, known["img00"], "oldest images are pruned when nothing is protected")
	assert.False(t, known["img01"])
	assert.False(t, known["img02"])
	assert.True(t, known["img03"])
	assert.True(t, known["img04"])
}

// TestSnapshotProtectedIDs_NilCallback verifies the helper tolerates an unset
// callback, which is the state in every test that does not opt in.
func TestSnapshotProtectedIDs_NilCallback(t *testing.T) {
	store := NewImageStore()
	assert.Nil(t, store.snapshotProtectedIDs())
}
