package wallpaper

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util/log"
)

// ImageStore is a thread-safe storage for wallpaper images.
// It manages the list of available images, seen history, and avoids blocklisted images.
type ImageStore struct {
	mu     sync.RWMutex
	images []provider.Image
	// idSet tracks image IDs for O(1) existence checks
	idSet    map[string]bool
	seen     map[string]bool
	avoidSet map[string]bool

	// cachePath is the path to the persistence file
	cachePath string
	fm        *FileManager
	asyncSave bool // Default true used during initialization? No, default bool is false.
	// We want default TRUE. So we need to init it.

	// Persistence Debouncing
	saveTimer *time.Timer
	saveMu    sync.Mutex // Protects timer access

	// Hook for testing
	saveFunc func()

	debounceDuration time.Duration
}

// NewImageStore creates a new ImageStore.
func NewImageStore() *ImageStore {
	s := &ImageStore{
		images:           make([]provider.Image, 0),
		idSet:            make(map[string]bool),
		seen:             make(map[string]bool),
		avoidSet:         make(map[string]bool),
		asyncSave:        true,
		debounceDuration: 2 * time.Second,
	}
	s.saveFunc = s.saveCacheInternalOriginalLocked
	return s
}

// SetDebounceDuration sets the debounce duration for persistence.
func (s *ImageStore) SetDebounceDuration(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debounceDuration = d
}

// SetAsyncSave enables or disables asynchronous saving.
// Useful for testing to prevent race conditions.
func (s *ImageStore) SetAsyncSave(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asyncSave = enabled
}

// SetFileManager sets the file manager and cache path.
func (s *ImageStore) SetFileManager(fm *FileManager, cacheFile string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fm = fm
	s.cachePath = cacheFile
}

// Update updates an existing image in the store.
// Returns true if the image was found and updated.
func (s *ImageStore) Update(img provider.Image) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find index by ID
	for i, existing := range s.images {
		if existing.ID == img.ID {
			s.images[i] = img
			if s.asyncSave {
				s.scheduleSaveLocked()
			}
			return true
		}
	}
	return false
}

// scheduleSaveLocked schedules a save operation.
// Caller must hold lock (though here we only access saveTimer protected by saveMu? No, let's use saveMu).
// Actually, to prevent deadlock, we should release s.mu before calling scheduleSave if scheduleSave takes saveMu.
// But Update holds s.mu.
// Let's make scheduleSave take s.mu? No, timer is separate concern.
// Just use a separate mutex for timer logic.
func (s *ImageStore) scheduleSaveLocked() {
	// We can run this in a goroutine to avoid holding s.mu while acquiring saveMu
	go func() {
		s.saveMu.Lock()
		defer s.saveMu.Unlock()

		if s.saveTimer != nil {
			s.saveTimer.Stop()
		}
		s.saveTimer = time.AfterFunc(s.debounceDuration, func() {
			s.saveCacheInternal()
		})
	}()
}

// Add adds a new image to the store if it hasn't been seen or avoided.
// Returns true if the image was added.
// It takes a Write Lock.
// Automatically triggers SaveCache if fm is set.
func (s *ImageStore) Add(img provider.Image) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check avoid set
	if s.avoidSet[img.ID] {
		return false
	}

	// Check if already exists using O(1) map
	if s.idSet[img.ID] {
		return false
	}

	s.images = append(s.images, img)
	s.idSet[img.ID] = true

	// Trigger save. If asyncSave is true, debounce it.
	if s.fm != nil {
		if s.asyncSave {
			s.scheduleSaveLocked()
		} else {
			// Sync save (still internal logic, but blocks Add)
			// We cannot call saveCacheInternal directly because it takes Lock.
			// We already hold Lock!
			// We need saveCacheInternalOriginalLocked.
			s.saveCacheInternalOriginalLocked()
		}
	}

	return true
}

// Get returns the image at the specified index.
// It takes a Read Lock.
func (s *ImageStore) Get(index int) (provider.Image, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if index >= 0 && index < len(s.images) {
		return s.images[index], true
	}
	return provider.Image{}, false
}

// GetByID returns the image with the specified ID.
// It takes a Read Lock.
func (s *ImageStore) GetByID(id string) (provider.Image, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.idSet[id] {
		return provider.Image{}, false
	}

	for _, img := range s.images {
		if img.ID == id {
			return img, true
		}
	}
	return provider.Image{}, false
}

// Contains checks if an image ID exists in the store.
func (s *ImageStore) Contains(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.idSet[id]
}

// Count returns the number of images.
func (s *ImageStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.images)
}

// MarkSeen marks an image as seen.
// It takes a Write Lock.
func (s *ImageStore) MarkSeen(filePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[filePath] = true
	log.Printf("[DEBUG-DUMP] Store: MarkSeen called for '%s'. Total Seen: %d", filePath, len(s.seen))
}

// Remove deletes an image by ID (or index) from the store and blocklists it.
// Returns true if found and removed.
// It takes a Write Lock.
func (s *ImageStore) Remove(id string) (provider.Image, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.idSet[id] {
		return provider.Image{}, false
	}

	for i, img := range s.images {
		if img.ID == id {
			// Found
			s.avoidSet[id] = true
			delete(s.seen, img.FilePath)
			delete(s.idSet, id)

			// Remove from slice
			s.images = append(s.images[:i], s.images[i+1:]...)

			// Persistent Cleanup
			if s.fm != nil {
				// Deep delete files
				if err := s.fm.DeepDelete(id); err != nil {
					log.Printf("Store: Failed to deep delete image %s: %v", id, err)
				}
				// Save metadata change
				// Save metadata change
				if s.asyncSave {
					go s.saveCacheInternal()
				} else {
					s.saveCacheInternalOriginalLocked()
				}
			}

			return img, true
		}
	}
	return provider.Image{}, false
}

// SeenCount returns the number of seen images.
func (s *ImageStore) SeenCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.seen)
}

// Clear resets the store memory.
// It does NOT delete files from disk.
func (s *ImageStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.images = []provider.Image{}
	s.idSet = make(map[string]bool)
	s.seen = make(map[string]bool)
	s.avoidSet = make(map[string]bool)
}

// Wipe clears the store memory AND deletes all files from disk using CleanupOrphans.
// It effectively resets the application state.
func (s *ImageStore) Wipe() {
	s.Clear()
	// Trigger full cleanup by passing an empty map of known IDs.
	// This marks ALL files as "orphans" and deletes them.
	if s.fm != nil {
		go s.fm.CleanupOrphans(map[string]bool{})
	}
	s.SaveCache() // Save the empty state
}

// RemoveByQueryID removes all images associated with the given queryID.
// It deletes their files and removes them from the memory store.
func (s *ImageStore) RemoveByQueryID(queryID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newImages := make([]provider.Image, 0, len(s.images))
	idsToRemove := []string{}

	for _, img := range s.images {
		if img.SourceQueryID == queryID {
			idsToRemove = append(idsToRemove, img.ID)
		} else {
			newImages = append(newImages, img)
		}
	}

	// Update store
	s.images = newImages
	for _, id := range idsToRemove {
		delete(s.idSet, id)
		delete(s.seen, id)
		// We do NOT remove from AvoidSet, as blocks should persist unless explicitly cleared.
	}

	// Persist changes
	// We always want to save if we remove something.
	// But we are holding lock.
	s.saveCacheInternalOriginalLocked()

	// Trigger deletions in background
	if s.fm != nil && len(idsToRemove) > 0 {
		go func(ids []string) {
			for _, id := range ids {
				if err := s.fm.DeepDelete(id); err != nil {
					log.Printf("Store: Failed to delete image %s: %v", id, err)
				}
			}
		}(idsToRemove)
	}
}

// List returns a copy of all images (for debugging or shuffle generation).
// Use with caution on large sets.
func (s *ImageStore) List() []provider.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dest := make([]provider.Image, len(s.images))
	copy(dest, s.images)
	return dest
}

// --- Persistence & Sync ---

// LoadCache loads images from the JSON cache file.
func (s *ImageStore) LoadCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cachePath == "" {
		return fmt.Errorf("cache path not set")
	}

	file, err := os.Open(s.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cache yet, normal
		}
		return err
	}
	defer file.Close()

	var loadedImages []provider.Image
	if err := json.NewDecoder(file).Decode(&loadedImages); err != nil {
		return fmt.Errorf("failed to decode cache: %w", err)
	}

	// Populate store
	s.images = loadedImages
	s.idSet = make(map[string]bool)
	for _, img := range s.images {
		s.idSet[img.ID] = true
	}

	log.Printf("Store: Loaded %d images from cache.", len(s.images))
	return nil
}

// SaveCache forces a save of the cache to disk.
func (s *ImageStore) SaveCache() {
	s.saveCacheInternal()
}

// Sync validates the in-memory store against disk and config.
// It performs Self-Healing and Grooming.
// limit: Max number of images to keep. 0 = means MIN limit (50).
// activeQueryIDs: Map of query IDs that are allowed. If nil, filtering is skipped.
func (s *ImageStore) Sync(limit int, targetFlags map[string]bool, activeQueryIDs map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fm == nil {
		return
	}

	log.Printf("Store: Syncing... (Limit: %d)", limit)

	if limit <= 0 {
		limit = 50 // Enforce minimum
	}

	var validImages []provider.Image

	// 1. Validation Logic
	for _, img := range s.images {
		// Strict Cache Sync: Prune images from inactive queries
		if activeQueryIDs != nil && img.SourceQueryID != "" && !activeQueryIDs[img.SourceQueryID] {
			log.Printf("Sync: Strict Cache Pruning. Removing image %s from inactive query %s.", img.ID, img.SourceQueryID)
			// Deep Delete (Async not needed, we are in maintenance mode)
			// But for safety, lets queue it or do it here. Sync can be slow.
			// Let's queue strict deletes alongside grooming or do it now.
			// Doing it now ensures immediate compliance.
			if err := s.fm.DeepDelete(img.ID); err != nil {
				log.Printf("Sync: Failed to prune %s: %v", img.ID, err)
			}
			continue
		}

		// A. Check Master Existence
		masterPath := s.fm.GetMasterPath(img.ID, ".jpg") // Try .jpg default
		if _, err := os.Stat(masterPath); os.IsNotExist(err) {
			masterPath = s.fm.GetMasterPath(img.ID, ".png")
			if _, err := os.Stat(masterPath); os.IsNotExist(err) {
				// Master missing!
				log.Printf("Sync: Master missing for %s. Pruning.", img.ID)
				if err := s.fm.DeepDelete(img.ID); err != nil {
					log.Printf("Sync: Failed to deep delete %s: %v", img.ID, err)
				} // Cleanup leftovers
				continue
			}
		}

		// B. Check Flags (Versioning)
		match := true
		if len(targetFlags) > 0 {
			if len(img.ProcessingFlags) != len(targetFlags) {
				match = false
				log.Printf("[DEBUG-DUMP] Flag Mismatch (Len): Img=%d, Target=%d", len(img.ProcessingFlags), len(targetFlags))
			} else {
				for k, v := range targetFlags {
					if img.ProcessingFlags[k] != v {
						match = false
						log.Printf("[DEBUG-DUMP] Flag Mismatch (Key: %s): Img=%v, Target=%v", k, img.ProcessingFlags[k], v)
						break
					}
				}
			}
		}

		if !match {
			log.Printf("[DEBUG-DUMP] Sync: Config mismatch for %s. Pruning to trigger cleanup/reprocess.", img.ID)
			continue
		}

		validImages = append(validImages, img)
	}

	s.images = validImages
	log.Printf("[DEBUG-DUMP] Store Sync: Final Valid Images Count: %d. (Started with: %d)", len(s.images), len(s.idSet))

	// 2. Grooming Logic (FIFO)
	if len(s.images) > limit {
		excess := len(s.images) - limit
		log.Printf("Store: Grooming %d excess images.", excess)

		// Assuming appended = newer. Remove from start (Oldest).
		// Need to delete files for these excess images.
		toRemove := s.images[:excess]
		s.images = s.images[excess:]

		go func(list []provider.Image) {
			for _, img := range list {
				// Pacer for low priority (Nightly Groom)
				time.Sleep(100 * time.Millisecond)
				if err := s.fm.DeepDelete(img.ID); err != nil {
					log.Printf("Store: Failed to deep delete (grooming) %s: %v", img.ID, err)
				}
			}
		}(toRemove)
	}

	// Rebuild Map
	s.idSet = make(map[string]bool)
	for _, img := range s.images {
		s.idSet[img.ID] = true
	}

	s.saveCacheInternalOriginalLocked() // Save the clean state
}

// GetKnownIDs returns a set of all known image IDs.
// Used for Orphan Cleanup.
func (s *ImageStore) GetKnownIDs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make(map[string]bool, len(s.idSet))
	for k, v := range s.idSet {
		ids[k] = v
	}
	return ids
}

// saveCacheInternal saves the store to disk.
// Uses a read lock internally to allow concurrent verification/reads while saving.
func (s *ImageStore) saveCacheInternal() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.saveFunc != nil {
		s.saveFunc()
	}
}

func (s *ImageStore) saveCacheInternalOriginalLocked() {
	if s.cachePath == "" {
		return
	}

	// Create temp file
	tmp := s.cachePath + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		log.Printf("Store: Failed to create temp cache: %v", err)
		return
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s.images); err != nil {
		file.Close()
		log.Printf("Store: Failed to encode cache: %v", err)
		return
	}
	file.Close()

	// Atomic rename
	if err := os.Rename(tmp, s.cachePath); err != nil {
		log.Printf("Store: Failed to save cache: %v", err)
	}
}

// DumpState prints a detailed dump of the store's internal state for debugging.
func (s *ImageStore) DumpState() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Printf("=== STORE STATE DUMP ===")
	log.Printf("Total Images: %d", len(s.images))
	for i, img := range s.images {
		seen := s.seen[img.FilePath]
		log.Printf("  [%d] ID: %s | Source: %s | Seen: %v | Path: %s", i, img.ID, img.SourceQueryID, seen, img.FilePath)
	}
	log.Printf("Total Seen Entries: %d", len(s.seen))
	log.Printf("Seen Map Dump:")
	for k := range s.seen {
		log.Printf("  - %s", k)
	}
	log.Printf("========================")
}
