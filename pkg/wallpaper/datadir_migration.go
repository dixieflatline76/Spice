package wallpaper

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/util/fsx"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// dataDirMigrationKey guards the one-shot relocation of the image tree out of
// the purgeable cache directory.
const dataDirMigrationKey = "wallpaper_migration_datadir_v1_done"

// downloadsDirName is the image tree's directory name under whichever root is
// in use.
var downloadsDirName = strings.ToLower(pluginName) + "_downloads"

// legacyDownloadsPath returns the pre-migration location of the image tree.
func legacyDownloadsPath() string {
	return filepath.Join(config.GetWorkingDir(), downloadsDirName)
}

// currentDownloadsPath returns the durable location of the image tree.
func currentDownloadsPath() string {
	return filepath.Join(config.GetDataDir(), downloadsDirName)
}

// migrateDataDir moves the image tree from the cache directory to the durable
// data directory, once. It returns the old root when a move actually happened,
// so the caller can rewrite the absolute paths held in the image cache.
func migrateDataDir(cfg *Config) (movedFrom string) {
	return migrateDataDirBetween(cfg, legacyDownloadsPath(), currentDownloadsPath())
}

// migrateDataDirBetween is the testable core of migrateDataDir.
//
// The move is an os.Rename between two directories under $HOME on the same
// APFS volume, which is an instant metadata operation regardless of how many
// gigabytes the tree holds. If it fails there is deliberately no copy
// fallback: silently duplicating several gigabytes at startup is worse than
// starting fresh, so we log loudly and leave the old tree for the user.
func migrateDataDirBetween(cfg *Config, oldRoot, newRoot string) (movedFrom string) {
	if oldRoot == newRoot {
		// Windows and Linux: the data dir is the working dir, nothing to do.
		return ""
	}

	if cfg != nil && cfg.BoolWithFallback(dataDirMigrationKey, false) {
		return ""
	}

	markDone := func() {
		if cfg != nil {
			cfg.SetBool(dataDirMigrationKey, true)
		}
	}

	if _, err := os.Stat(oldRoot); err != nil {
		// Nothing to migrate. This is the normal path for a fresh install, and
		// for the sandboxed App Store build, whose container makes the legacy
		// location invisible: it simply starts out in the new location.
		markDone()
		return ""
	}

	if _, err := os.Stat(newRoot); err == nil {
		// A previous run already populated the destination. Do not merge trees.
		log.Printf("[Migration] Data directory %s already exists; leaving legacy tree at %s untouched.", newRoot, oldRoot)
		markDone()
		return ""
	}

	if err := os.MkdirAll(filepath.Dir(newRoot), 0755); err != nil {
		log.Printf("[Migration] Could not create data directory %s: %v. Keeping images in the cache directory.", filepath.Dir(newRoot), err)
		return ""
	}

	if err := os.Rename(oldRoot, newRoot); err != nil {
		log.Printf("[Migration] Could not move image data from %s to %s: %v. Starting fresh in the new location; the old files are left in place and can be deleted manually.", oldRoot, newRoot, err)
		markDone()
		return ""
	}

	log.Printf("[Migration] Moved image data from %s to %s so wallpapers survive a reboot.", oldRoot, newRoot)
	markDone()
	return oldRoot
}

// excludeDataDirFromBackup marks the image tree so Time Machine skips it.
// Best-effort: a failure here is logged and otherwise ignored.
func excludeDataDirFromBackup(path string) {
	if err := fsx.ExcludeFromBackup(path); err != nil {
		log.Debugf("[Migration] Could not exclude %s from backups: %v", path, err)
	}
}
