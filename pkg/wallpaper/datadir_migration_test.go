package wallpaper

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
)

// TestMigrateRoot_RewritesAbsolutePaths is the migration's real hazard: the
// cache stores absolute paths, so moving the tree on disk invalidates every
// one of them. Without the rewrite the whole library looks missing and is
// re-downloaded.
func TestMigrateRoot_RewritesAbsolutePaths(t *testing.T) {
	oldRoot := "/Users/dev/Library/Caches/spice/wallpaper_downloads"
	newRoot := "/Users/dev/Library/Application Support/Spice/data/wallpaper_downloads"

	store := NewImageStore()
	store.SetAsyncSave(false)
	store.Add(provider.Image{
		ID:       "img1",
		FilePath: filepath.Join(oldRoot, "img1.jpg"),
		DerivativePaths: map[string]string{
			"1920x1080": filepath.Join(oldRoot, "fitted", "quality", "standard", "1920x1080", "img1.jpg"),
			"3440x1440": filepath.Join(oldRoot, "fitted", "quality", "standard", "3440x1440", "img1.jpg"),
		},
	})

	assert.True(t, store.MigrateRoot(oldRoot, newRoot))

	list := store.List()
	assert.Len(t, list, 1)
	assert.Equal(t, filepath.Join(newRoot, "img1.jpg"), list[0].FilePath)
	for res, path := range list[0].DerivativePaths {
		assert.Contains(t, path, newRoot, "derivative %s was not rewritten", res)
		assert.NotContains(t, path, oldRoot)
	}
}

// TestMigrateRoot_RebuildsPathIndex verifies MarkSeen still works after the
// rewrite; pathSet is keyed by FilePath and would otherwise be stale.
func TestMigrateRoot_RebuildsPathIndex(t *testing.T) {
	oldRoot := "/old/root"
	newRoot := "/new/root"

	store := NewImageStore()
	store.SetAsyncSave(false)
	store.Add(provider.Image{ID: "img1", FilePath: filepath.Join(oldRoot, "img1.jpg")})

	assert.True(t, store.MigrateRoot(oldRoot, newRoot))

	store.MarkSeen(filepath.Join(newRoot, "img1.jpg"))
	assert.Equal(t, 1, store.SeenCount(), "MarkSeen must resolve the rewritten path")
}

// TestMigrateRoot_LeavesForeignPathsAlone verifies only paths under the old
// root are touched.
func TestMigrateRoot_LeavesForeignPathsAlone(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.Add(provider.Image{ID: "outside", FilePath: "/somewhere/else/img.jpg"})

	assert.False(t, store.MigrateRoot("/old/root", "/new/root"))
	assert.Equal(t, "/somewhere/else/img.jpg", store.List()[0].FilePath)
}

// TestMigrateRoot_NoopArguments covers the degenerate inputs.
func TestMigrateRoot_NoopArguments(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.Add(provider.Image{ID: "img1", FilePath: "/old/root/img1.jpg"})

	assert.False(t, store.MigrateRoot("", "/new/root"))
	assert.False(t, store.MigrateRoot("/old/root", ""))
	assert.False(t, store.MigrateRoot("/old/root", "/old/root"))
	assert.Equal(t, "/old/root/img1.jpg", store.List()[0].FilePath)
}

// TestMigrateRoot_EndToEndWithRealFiles exercises the full sequence: move the
// tree, rewrite the cache, and confirm the recorded paths point at files that
// actually exist.
func TestMigrateRoot_EndToEndWithRealFiles(t *testing.T) {
	base := t.TempDir()
	oldRoot := filepath.Join(base, "Caches", "spice", "wallpaper_downloads")
	newRoot := filepath.Join(base, "Application Support", "Spice", "data", "wallpaper_downloads")

	derivDir := filepath.Join(oldRoot, FittedRootDir, QualityDir, StandardDir, "1920x1080")
	assert.NoError(t, os.MkdirAll(derivDir, 0755))

	masterPath := filepath.Join(oldRoot, "img1.jpg")
	derivPath := filepath.Join(derivDir, "img1.jpg")
	assert.NoError(t, os.WriteFile(masterPath, []byte("master"), 0644))
	assert.NoError(t, os.WriteFile(derivPath, []byte("deriv"), 0644))

	store := NewImageStore()
	store.SetAsyncSave(false)
	store.Add(provider.Image{
		ID:              "img1",
		FilePath:        masterPath,
		DerivativePaths: map[string]string{"1920x1080": derivPath},
	})

	// Move the tree exactly as migrateDataDir does.
	assert.NoError(t, os.MkdirAll(filepath.Dir(newRoot), 0755))
	assert.NoError(t, os.Rename(oldRoot, newRoot))

	assert.True(t, store.MigrateRoot(oldRoot, newRoot))

	migrated := store.List()[0]
	assert.FileExists(t, migrated.FilePath, "rewritten master path must resolve")
	assert.FileExists(t, migrated.DerivativePaths["1920x1080"], "rewritten derivative path must resolve")
	assert.NoDirExists(t, oldRoot)
}

// migrationRoots returns an isolated old/new pair under a temp dir so these
// tests never depend on the real cache directory of the machine running them.
func migrationRoots(t *testing.T) (oldRoot, newRoot string) {
	t.Helper()
	base := t.TempDir()
	return filepath.Join(base, "Caches", "spice", downloadsDirName),
		filepath.Join(base, "AppSupport", "Spice", "data", downloadsDirName)
}

// migrationConfig returns the config with the one-shot flag reset. GetConfig is
// a process-wide singleton, so without this the flag leaks between tests and
// the result depends on execution order.
func migrationConfig(t *testing.T, done bool) *Config {
	t.Helper()
	cfg := GetConfig(NewMemoryPreferences())
	cfg.SetBool(dataDirMigrationKey, done)
	return cfg
}

// TestMigrateDataDir_MovesTree is the happy path: the tree lands in the durable
// location and the caller is told the old root so it can rewrite the cache.
func TestMigrateDataDir_MovesTree(t *testing.T) {
	cfg := migrationConfig(t, false)
	oldRoot, newRoot := migrationRoots(t)

	assert.NoError(t, os.MkdirAll(oldRoot, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(oldRoot, "img1.jpg"), []byte("data"), 0644))

	assert.Equal(t, oldRoot, migrateDataDirBetween(cfg, oldRoot, newRoot))

	assert.FileExists(t, filepath.Join(newRoot, "img1.jpg"))
	assert.NoDirExists(t, oldRoot)
	assert.True(t, cfg.BoolWithFallback(dataDirMigrationKey, false))
}

// TestMigrateDataDir_MarksDoneWhenNothingToMove verifies the one-shot flag is
// set even on a fresh install, so the check does not run every launch.
func TestMigrateDataDir_MarksDoneWhenNothingToMove(t *testing.T) {
	cfg := migrationConfig(t, false)
	oldRoot, newRoot := migrationRoots(t)

	assert.Equal(t, "", migrateDataDirBetween(cfg, oldRoot, newRoot))
	assert.True(t, cfg.BoolWithFallback(dataDirMigrationKey, false), "migration should be marked done")
}

// TestMigrateDataDir_DoesNotMergeIntoExistingDestination guards the user's data:
// if both trees exist, leave them alone rather than merging or clobbering.
func TestMigrateDataDir_DoesNotMergeIntoExistingDestination(t *testing.T) {
	cfg := migrationConfig(t, false)
	oldRoot, newRoot := migrationRoots(t)

	assert.NoError(t, os.MkdirAll(oldRoot, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(oldRoot, "old.jpg"), []byte("old"), 0644))
	assert.NoError(t, os.MkdirAll(newRoot, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(newRoot, "new.jpg"), []byte("new"), 0644))

	assert.Equal(t, "", migrateDataDirBetween(cfg, oldRoot, newRoot))

	assert.FileExists(t, filepath.Join(oldRoot, "old.jpg"), "legacy tree must be left intact")
	assert.FileExists(t, filepath.Join(newRoot, "new.jpg"))
	assert.NoFileExists(t, filepath.Join(newRoot, "old.jpg"), "trees must not be merged")
	assert.True(t, cfg.BoolWithFallback(dataDirMigrationKey, false))
}

// TestMigrateDataDir_SkipsWhenAlreadyDone verifies the guard short-circuits,
// leaving even a present legacy tree untouched on later launches.
func TestMigrateDataDir_SkipsWhenAlreadyDone(t *testing.T) {
	cfg := migrationConfig(t, true)
	oldRoot, newRoot := migrationRoots(t)

	assert.NoError(t, os.MkdirAll(oldRoot, 0755))

	assert.Equal(t, "", migrateDataDirBetween(cfg, oldRoot, newRoot))
	assert.DirExists(t, oldRoot)
	assert.NoDirExists(t, newRoot)
}

// TestMigrateRoot_IsIdempotentAndSelfHealing covers the crash-between-steps
// case: the files are moved in Init and the paths rewritten in Activate, with
// the one-shot flag already consumed in between. If the process dies after the
// move, the next launch must still repair the stranded paths, so the rewrite
// has to be safe to run on every activation.
func TestMigrateRoot_IsIdempotentAndSelfHealing(t *testing.T) {
	oldRoot := "/old/root"
	newRoot := "/new/root"

	store := NewImageStore()
	store.SetAsyncSave(false)
	store.Add(provider.Image{
		ID:              "stranded",
		FilePath:        filepath.Join(oldRoot, "stranded.jpg"),
		DerivativePaths: map[string]string{"1920x1080": filepath.Join(oldRoot, "d", "stranded.jpg")},
	})

	// First activation after the crash repairs the paths.
	assert.True(t, store.MigrateRoot(oldRoot, newRoot))
	assert.Equal(t, filepath.Join(newRoot, "stranded.jpg"), store.List()[0].FilePath)

	// Every later activation is a no-op and must not corrupt the paths.
	assert.False(t, store.MigrateRoot(oldRoot, newRoot))
	assert.Equal(t, filepath.Join(newRoot, "stranded.jpg"), store.List()[0].FilePath)
	assert.Equal(t, filepath.Join(newRoot, "d", "stranded.jpg"), store.List()[0].DerivativePaths["1920x1080"])
}

// TestMigrateDataDir_NoopWhenRootsMatch covers Windows and Linux, where the
// data dir and the working dir are the same place.
func TestMigrateDataDir_NoopWhenRootsMatch(t *testing.T) {
	cfg := migrationConfig(t, false)
	root := filepath.Join(t.TempDir(), downloadsDirName)
	assert.NoError(t, os.MkdirAll(root, 0755))

	assert.Equal(t, "", migrateDataDirBetween(cfg, root, root))
	assert.DirExists(t, root)
	assert.False(t, cfg.BoolWithFallback(dataDirMigrationKey, false),
		"a platform with nothing to migrate should not consume the one-shot flag")
}
