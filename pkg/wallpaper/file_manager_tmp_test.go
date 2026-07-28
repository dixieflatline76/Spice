package wallpaper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBaseIDFromFileName(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		wantID    string
		wantIsTmp bool
	}{
		{"master jpg", "Wikimedia_123.jpg", "Wikimedia_123", false},
		{"master jpeg", "Pexels_9.jpeg", "Pexels_9", false},
		{"master png", "Wikimedia_123.png", "Wikimedia_123", false},
		{"hardlink alias", "Wikimedia_123.jpg.spice_tmp", "Wikimedia_123", true},
		{"hardlink alias png", "Wikimedia_123.png.spice_tmp", "Wikimedia_123", true},
		{"uppercase alias suffix", "Wikimedia_123.jpg.SPICE_TMP", "Wikimedia_123", true},
		{"id containing dots", "some.id.v2.jpg", "some.id.v2", false},
		{"id containing dots as alias", "some.id.v2.jpg.spice_tmp", "some.id.v2", true},
		{"no extension", "plainfile", "plainfile", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, isTmp := baseIDFromFileName(tt.fileName)
			assert.Equal(t, tt.wantID, id)
			assert.Equal(t, tt.wantIsTmp, isTmp)
		})
	}
}

func TestIsImageOrTmp(t *testing.T) {
	assert.True(t, isImageOrTmp("a.jpg"))
	assert.True(t, isImageOrTmp("a.JPEG"))
	assert.True(t, isImageOrTmp("a.png"))
	assert.True(t, isImageOrTmp("a.jpg.spice_tmp"))
	assert.False(t, isImageOrTmp("image_cache_map.json"))
	assert.False(t, isImageOrTmp("a.txt"))
	assert.False(t, isImageOrTmp("a.txt.spice_tmp"))
}

// writeAged creates a file and backdates its mtime, so age-based rules can be
// exercised without sleeping.
func writeAged(t *testing.T, path string, age time.Duration) {
	t.Helper()
	assert.NoError(t, os.WriteFile(path, []byte("data"), 0644))
	when := time.Now().Add(-age)
	assert.NoError(t, os.Chtimes(path, when, when))
}

// TestCleanupOrphans_RemovesStaleTmpAlias covers the disk leak: these hardlinks
// were invisible to the sweep because filepath.Ext sees ".spice_tmp", so they
// accumulated and pinned deleted image data on disk forever.
func TestCleanupOrphans_RemovesStaleTmpAlias(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	derivDir := filepath.Join(FittedRootDir, FlexibilityDir, FaceCropDir)
	base, err := fm.GetDerivativePath("valid", ".jpg", derivDir)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(base, []byte("data"), 0644))

	// An alias older than the grace period whose owner is still known.
	stale := base + SpiceTmpSuffix
	writeAged(t, stale, 30*time.Minute)

	fm.CleanupOrphans(map[string]bool{"valid": true}, nil)

	assert.NoFileExists(t, stale, "stale hardlink alias should be reclaimed")
	assert.FileExists(t, base, "the real derivative must be kept")
}

// TestCleanupOrphans_RemovesTmpAliasWithMissingBase verifies an alias is dropped
// as soon as the file it aliases is gone, regardless of age.
func TestCleanupOrphans_RemovesTmpAliasWithMissingBase(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	derivDir := filepath.Join(FittedRootDir, FlexibilityDir, FaceCropDir)
	base, err := fm.GetDerivativePath("valid", ".jpg", derivDir)
	assert.NoError(t, err)

	// Alias only; the base file was already deleted.
	orphanAlias := base + SpiceTmpSuffix
	writeAged(t, orphanAlias, time.Second)

	fm.CleanupOrphans(map[string]bool{"valid": true}, nil)

	assert.NoFileExists(t, orphanAlias, "alias without a base file should be reclaimed")
}

// TestCleanupOrphans_RemovesTmpAliasOfUnknownID verifies an alias goes away with
// its image when the ID leaves the store.
func TestCleanupOrphans_RemovesTmpAliasOfUnknownID(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	derivDir := filepath.Join(FittedRootDir, FlexibilityDir, FaceCropDir)
	base, err := fm.GetDerivativePath("gone", ".jpg", derivDir)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(base, []byte("data"), 0644))
	alias := base + SpiceTmpSuffix
	writeAged(t, alias, time.Second)

	fm.CleanupOrphans(map[string]bool{"other": true}, nil)

	assert.NoFileExists(t, base)
	assert.NoFileExists(t, alias)
}

// TestCleanupOrphans_KeepsProtectedTmpAlias is the safety counterpart: when the
// wallpaper in effect IS the alias URL, deleting it blanks the desktop.
func TestCleanupOrphans_KeepsProtectedTmpAlias(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	derivDir := filepath.Join(FittedRootDir, FlexibilityDir, FaceCropDir)
	base, err := fm.GetDerivativePath("onscreen", ".jpg", derivDir)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(base, []byte("data"), 0644))

	alias := base + SpiceTmpSuffix
	writeAged(t, alias, 30*time.Minute)

	// Protected even though the ID is not in knownIDs at all.
	fm.CleanupOrphans(map[string]bool{}, map[string]bool{"onscreen": true})

	assert.FileExists(t, alias, "the alias backing the live wallpaper must be kept")
	assert.FileExists(t, base, "the protected derivative must be kept")
}

// TestCleanupOrphans_KeepsFreshTmpAlias verifies the grace period, which avoids
// racing a wallpaper change still in flight on the macOS main thread.
func TestCleanupOrphans_KeepsFreshTmpAlias(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	derivDir := filepath.Join(FittedRootDir, FlexibilityDir, FaceCropDir)
	base, err := fm.GetDerivativePath("valid", ".jpg", derivDir)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(base, []byte("data"), 0644))

	fresh := base + SpiceTmpSuffix
	writeAged(t, fresh, time.Second)

	fm.CleanupOrphans(map[string]bool{"valid": true}, nil)

	assert.FileExists(t, fresh, "a just-created alias must survive the grace period")
}

// TestCleanupOrphans_ProtectsMasterOfOnScreenImage verifies protection also
// covers plain image files, not just aliases.
func TestCleanupOrphans_ProtectsMasterOfOnScreenImage(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	master := filepath.Join(tmpDir, "onscreen.jpg")
	assert.NoError(t, os.WriteFile(master, []byte("data"), 0644))

	// knownIDs is empty, so without protection this master would be deleted.
	fm.CleanupOrphans(map[string]bool{}, map[string]bool{"onscreen": true})

	assert.FileExists(t, master)
}

// TestDeepDeleteBatch_RemovesTmpAlias verifies the batch delete no longer
// leaves an alias behind. Because the alias is a hardlink, an orphaned one
// keeps the deleted image's data allocated on disk.
func TestDeepDeleteBatch_RemovesTmpAlias(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	master := filepath.Join(tmpDir, "doomed.jpg")
	assert.NoError(t, os.WriteFile(master, []byte("data"), 0644))
	masterAlias := master + SpiceTmpSuffix
	assert.NoError(t, os.WriteFile(masterAlias, []byte("data"), 0644))

	derivDir := filepath.Join(FittedRootDir, FlexibilityDir, FaceCropDir)
	deriv, err := fm.GetDerivativePath("doomed", ".jpg", derivDir)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(deriv, []byte("data"), 0644))
	derivAlias := deriv + SpiceTmpSuffix
	assert.NoError(t, os.WriteFile(derivAlias, []byte("data"), 0644))

	assert.NoError(t, fm.DeepDeleteBatch([]string{"doomed"}))

	assert.NoFileExists(t, master)
	assert.NoFileExists(t, masterAlias)
	assert.NoFileExists(t, deriv)
	assert.NoFileExists(t, derivAlias)
}

// TestDeleteDerivatives_RemovesTmpAlias verifies invalidation also clears the
// aliases, otherwise a stale alias can be re-served after reprocessing.
func TestDeleteDerivatives_RemovesTmpAlias(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	master := filepath.Join(tmpDir, "img.jpg")
	assert.NoError(t, os.WriteFile(master, []byte("data"), 0644))

	derivDir := filepath.Join(FittedRootDir, FlexibilityDir, FaceCropDir)
	deriv, err := fm.GetDerivativePath("img", ".jpg", derivDir)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(deriv, []byte("data"), 0644))
	alias := deriv + SpiceTmpSuffix
	assert.NoError(t, os.WriteFile(alias, []byte("data"), 0644))

	assert.NoError(t, fm.DeleteDerivatives("img"))

	assert.NoFileExists(t, deriv)
	assert.NoFileExists(t, alias)
	assert.FileExists(t, master, "DeleteDerivatives must keep the master")
}

// TestRenameAllAssets_SkipsTmpAlias verifies renaming does not carry a
// disposable alias to the new ID, leaving it to be reclaimed as an orphan.
func TestRenameAllAssets_SkipsTmpAlias(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	assert.NoError(t, fm.EnsureDirs())

	master := filepath.Join(tmpDir, "old.jpg")
	assert.NoError(t, os.WriteFile(master, []byte("data"), 0644))
	alias := master + SpiceTmpSuffix
	assert.NoError(t, os.WriteFile(alias, []byte("data"), 0644))

	assert.NoError(t, fm.RenameAllAssets("old", "new"))

	assert.FileExists(t, filepath.Join(tmpDir, "new.jpg"), "master should be renamed")
	assert.NoFileExists(t, master)
	assert.FileExists(t, alias, "the alias is left behind for the orphan sweep")
	assert.NoFileExists(t, filepath.Join(tmpDir, "new.jpg"+SpiceTmpSuffix))

	// The orphan sweep then reclaims it, since "old" is no longer a known ID.
	when := time.Now().Add(-30 * time.Minute)
	assert.NoError(t, os.Chtimes(alias, when, when))
	fm.CleanupOrphans(map[string]bool{"new": true}, nil)
	assert.NoFileExists(t, alias)
}
