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
			} else {
				s.saveCacheInternalOriginalLocked()
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

}

// Remove deletes an image by ID (or index) from the store and blocklists it.
// Returns true if found and removed.
// It takes a Write Lock.
// Remove deletes an image by ID (or index) from the store and blocklists it.
// Returns true if found and removed.
// It takes a Write Lock for memory updates, then releases it for I/O.
func (s *ImageStore) Remove(id string) (provider.Image, bool) {
	s.mu.Lock()

	// 1. Check & Memory Update
	if !s.idSet[id] {
		s.mu.Unlock()
		return provider.Image{}, false
	}

	var removedImg provider.Image
	found := false

	for i, img := range s.images {
		if img.ID == id {
			removedImg = img
			found = true

			// Update Memory State
			s.avoidSet[id] = true
			delete(s.seen, img.FilePath)
			delete(s.idSet, id)
			s.images = append(s.images[:i], s.images[i+1:]...)
			break
		}
	}
	s.mu.Unlock() // Release lock immediately after memory update

	if !found {
		return provider.Image{}, false
	}

	// 2. Disk I/O (Outside Lock)
	if s.fm != nil {
		if err := s.fm.DeepDelete(id); err != nil {
			log.Printf("Store: Failed to deep delete image %s: %v", id, err)
		}
		// Save changes
		s.SaveCache()
	}

	return removedImg, true
}

// SeenCount returns the number of seen images.
func (s *ImageStore) SeenCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.seen)
}

// Clear resets the store memory (Images, IDSet, Seen).
// It does NOT clear the AvoidSet (Blocklist).
func (s *ImageStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.images = []provider.Image{}
	s.idSet = make(map[string]bool)
	s.seen = make(map[string]bool)
	// Do NOT clear avoidSet here. It should persist across refreshes.
}

// Wipe clears the store memory AND deletes all files from disk using CleanupOrphans.
// It effectively resets the application state.
func (s *ImageStore) Wipe() {
	s.Clear()

	s.mu.Lock()
	s.avoidSet = make(map[string]bool) // Explicitly clear avoidSet on Wipe
	s.mu.Unlock()

	// Trigger full cleanup by passing an empty map of known IDs.
	// Trigger full cleanup by passing an empty map of known IDs.
	// This marks ALL files as "orphans" and deletes them.
	if s.fm != nil {
		go s.fm.CleanupOrphans(map[string]bool{})
	}
	s.SaveCache() // Save the empty state
}

// RemoveByQueryID removes all images associated with the given queryID.
// It deletes their files and removes them from the memory store.
// RemoveByQueryID removes all images associated with the given queryID.
// It deletes their files and removes them from the memory store.
func (s *ImageStore) RemoveByQueryID(queryID string) {
	s.mu.Lock()

	newImages := make([]provider.Image, 0, len(s.images))
	idsToRemove := []string{}

	// 1. Memory Filter
	for _, img := range s.images {
		if img.SourceQueryID == queryID {
			idsToRemove = append(idsToRemove, img.ID)
			delete(s.idSet, img.ID)
			delete(s.seen, img.ID) // Bug fix: previously using ID for seen map? Wait, seen map uses FilePath usually.
			// Let's check MarkSeen: s.seen[filePath] = true.
			// So verify if we should delete by ID or FilePath.
			// Ideally we should delete by FilePath.
			// In original code: delete(s.seen, id). That looks like a bug in original code too?
			// Let's check original.
			// Original Line 306: delete(s.seen, id).
			// Yes, seems like a bug. MarkSeen uses FilePath.
			// I should probably fix it to delete(s.seen, img.FilePath).
			delete(s.seen, img.FilePath)
		} else {
			newImages = append(newImages, img)
		}
	}
	s.images = newImages
	s.mu.Unlock() // Release Lock

	// 2. Disk I/O (Outside Lock)
	if len(idsToRemove) > 0 {
		// Save first to persist the removal from list
		s.SaveCache()

		// Then delete files
		if s.fm != nil {
			go func(ids []string) {
				for _, id := range ids {
					if err := s.fm.DeepDelete(id); err != nil {
						log.Printf("Store: Failed to delete image %s: %v", id, err)
					}
				}
			}(idsToRemove)
		}
	}
}

// List returns a copy of all images (for debugging or shuffle generation).
// Use with caution on large sets.
func (s *ImageStore) List() []provider.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dest := make([]provider.Image, len(s.images))
	copy(dest, s.images)
	copy(dest, s.images)
	return dest
}

// LoadAvoidSet populates the avoid set from a map.
func (s *ImageStore) LoadAvoidSet(avoidSet map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range avoidSet {
		s.avoidSet[id] = true
	}
	log.Debugf("Store: Loaded %d blocked images from config.", len(avoidSet))
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

	log.Debugf("Store: Loaded %d images from cache.", len(s.images))
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
// Sync validates the in-memory store against disk and config.
// Optimized: Performs I/O (os.Stat) without holding main lock.
func (s *ImageStore) Sync(limit int, targetFlags map[string]bool, activeQueryIDs map[string]bool) {
	if s.fm == nil {
		return
	}

	log.Debugf("Store: Syncing... (Limit: %d)", limit)
	if limit <= 0 {
		limit = 50
	}

	// 1. Snapshot for Validation (Read Lock)
	s.mu.RLock()
	candidates := make([]provider.Image, len(s.images))
	copy(candidates, s.images)
	s.mu.RUnlock()

	// 2. Validation (No Lock - Heavy I/O)
	// We identify IDs that fail validation.
	// badIDs maps ID to Action: true=Delete(Deep), false=Forget(MemoryOnly)
	badIDs := make(map[string]bool)

	for _, img := range candidates {
		// A. Strict Cache Sync (Config Check - Fast)
		if activeQueryIDs != nil && img.SourceQueryID != "" && !activeQueryIDs[img.SourceQueryID] {
			log.Printf("Sync: Strict Cache Pruning. Removing image %s from inactive query %s.", img.ID, img.SourceQueryID)
			badIDs[img.ID] = true
			continue
		}

		// B. Master Existence (Disk Check - Slow)
		// Try .jpg
		masterPath := s.fm.GetMasterPath(img.ID, ".jpg")
		if _, err := os.Stat(masterPath); os.IsNotExist(err) {
			// Try .png
			masterPath = s.fm.GetMasterPath(img.ID, ".png")
			if _, err := os.Stat(masterPath); os.IsNotExist(err) {
				log.Debugf("Sync: Master missing for ID %s. Pruning.", img.ID)
				badIDs[img.ID] = true
				continue
			}
		}

		// C. Flag Mismatch (Logic Check - Fast)
		if len(targetFlags) > 0 {
			match := true
			if len(img.ProcessingFlags) != len(targetFlags) {
				match = false
			} else {
				for k, v := range targetFlags {
					if img.ProcessingFlags[k] != v {
						match = false
						break
					}
				}
			}
			if !match {
				log.Debugf("Sync: Config mismatch for %s. Pruning.", img.ID)
				badIDs[img.ID] = false // Forget Only! Keep Master for reprocessing.
			}
		}
	}

	// 3. Update & Groom (Write Lock)
	s.mu.Lock()
	var finalImages []provider.Image
	var idsToDelete []string

	// Filter out bad IDs
	for _, img := range s.images {
		// New Check: make sure it hasn't been removed by concurrent Remove()
		// And check if it was marked bad by us.
		if deleteFile, shouldRemove := badIDs[img.ID]; shouldRemove {
			if deleteFile && !img.IsFavorited {
				idsToDelete = append(idsToDelete, img.ID)
			} else if deleteFile && img.IsFavorited {
				log.Debugf("Sync: Spared favorited image %s from deletion.", img.ID)
			}
			// Both actions remove from Memory Store
			delete(s.idSet, img.ID)
		} else {
			finalImages = append(finalImages, img)
		}
	}

	// Grooming (FIFO - remove excess from start)
	if len(finalImages) > limit {
		excess := len(finalImages) - limit
		log.Debugf("Store: Grooming %d excess images.", excess)

		groomingCandidates := finalImages[:excess]
		finalImages = finalImages[excess:]

		for _, img := range groomingCandidates {
			idsToDelete = append(idsToDelete, img.ID)
			delete(s.idSet, img.ID)
		}
	}

	s.images = finalImages

	// Rebuild IDSet? We updated it incrementally above.
	// But let's verify consistency if needed.
	// The incremental delete(s.idSet, ...) is correct only if we processed ALL s.images.
	// We did.

	log.Debugf("Store Sync: Final Valid Images Count: %d. (Deleted: %d)", len(s.images), len(idsToDelete))
	s.mu.Unlock()

	// 4. Batch Delete Files (No Lock - Heavy I/O)
	if len(idsToDelete) > 0 {
		go func(ids []string) {
			for _, id := range ids {
				if err := s.fm.DeepDelete(id); err != nil {
					log.Printf("Store: Failed to deep delete %s: %v", id, err)
				}
			}
		}(idsToDelete)

		// Save the Clean State once
		s.SaveCache()
	} else if len(badIDs) > 0 {
		// Even if only "Forget" happened, we changed the store logic, so save.
		s.SaveCache()
	}
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
