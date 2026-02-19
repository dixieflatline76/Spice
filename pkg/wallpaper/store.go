package wallpaper

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util/log"
)

// ImageStore is a thread-safe storage for wallpaper images.
type ImageStore struct {
	mu       sync.RWMutex
	images   []provider.Image
	idSet    map[string]bool
	pathSet  map[string]int // filePath -> index for O(1) MarkSeen
	avoidSet map[string]bool

	seenCount int // Pre-calculated O(1) SeenCount

	cachePath string
	fm        *FileManager
	asyncSave bool

	saveTimer *time.Timer
	saveMu    sync.Mutex

	// Testing hook
	saveFunc func()

	// Update Notification
	updateCh chan struct{}

	// Resolution Buckets (Performance optimization)
	// Map "WidthxHeight" -> List of Image IDs compatible with that resolution
	resolutionBuckets map[string][]string

	debounceDuration time.Duration
	os               OS
}

func NewImageStore() *ImageStore {
	store := &ImageStore{
		images:            make([]provider.Image, 0),
		idSet:             make(map[string]bool),
		pathSet:           make(map[string]int),
		avoidSet:          make(map[string]bool),
		asyncSave:         true,
		debounceDuration:  2 * time.Second,
		updateCh:          make(chan struct{}),
		resolutionBuckets: make(map[string][]string),
	}
	return store
}

func (s *ImageStore) SetDebounceDuration(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debounceDuration = d
}

// SetOS sets the OS interface for filesystem operations.
func (s *ImageStore) SetOS(os OS) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.os = os
}

func (s *ImageStore) getStatFunc() func(string) (os.FileInfo, error) {
	if s.os != nil {
		return s.os.Stat
	}
	return os.Stat
}

func (s *ImageStore) SetAsyncSave(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asyncSave = enabled
}

func (s *ImageStore) SetFileManager(fm *FileManager, cacheFile string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fm = fm
	s.cachePath = cacheFile
}

func (s *ImageStore) Update(img provider.Image) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.images {
		if existing.ID == img.ID {
			s.images[i] = img
			s.rebuildBucketsLocked() // Rebuild because DerivativePaths might have changed
			s.scheduleSaveLocked()
			return true
		}
	}
	return false
}

// rebuildBucketsLocked re-scans all images and builds resolution buckets.
// CALLER MUST HOLD s.mu.Lock() or s.mu.RLock() - no, wait, it modifies buckets so must be WRITE LOCK.
func (s *ImageStore) rebuildBucketsLocked() {
	s.resolutionBuckets = make(map[string][]string)
	for _, img := range s.images {
		for res := range img.DerivativePaths {
			s.resolutionBuckets[res] = append(s.resolutionBuckets[res], img.ID)
		}
	}
}

// scheduleSaveLocked handles persistence.
// CALLER MUST HOLD s.mu.Lock()
func (s *ImageStore) scheduleSaveLocked() {
	if !s.asyncSave {
		// Sync mode: snapshot while locked and save immediately.
		snapshot := make([]provider.Image, len(s.images))
		copy(snapshot, s.images)
		s.saveCacheInternal(snapshot)
		return
	}

	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	s.saveTimer = time.AfterFunc(s.debounceDuration, func() {
		s.SaveCache()
	})
}

// Add adds a new image to the store. Returns true if added, false if it was already present or blocked.
func (s *ImageStore) Add(img provider.Image) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.idSet[img.ID]; exists {
		return false
	}
	if s.avoidSet[img.ID] {
		return false
	}

	s.images = append(s.images, img)
	s.idSet[img.ID] = true
	if img.FilePath != "" {
		s.pathSet[img.FilePath] = len(s.images) - 1
	}
	if img.Seen {
		s.seenCount++
	}

	// Add to buckets
	for res := range img.DerivativePaths {
		s.resolutionBuckets[res] = append(s.resolutionBuckets[res], img.ID)
	}

	s.scheduleSaveLocked()
	s.notifyUpdateLocked()
	return true
}

// Get returns the image at the given index from the global list. Returns false if index is out of bounds.
func (s *ImageStore) Get(index int) (provider.Image, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if index >= 0 && index < len(s.images) {
		return s.images[index], true
	}
	return provider.Image{}, false
}

// GetByID returns the image with the given ID. Returns false if not found.
func (s *ImageStore) GetByID(id string) (provider.Image, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, img := range s.images {
		if img.ID == id {
			return img, true
		}
	}
	return provider.Image{}, false
}

func (s *ImageStore) Contains(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.idSet[id]
}

// Count returns the total number of images in the store.
func (s *ImageStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.images)
}

func (s *ImageStore) MarkSeen(filePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, exists := s.pathSet[filePath]
	if !exists {
		// Fallback: This might happen if FilePath changed or wasn't indexed.
		// We still want to be robust but indexed is preferred.
		for i, img := range s.images {
			if img.FilePath == filePath {
				if !s.images[i].Seen {
					s.images[i].Seen = true
					s.seenCount++
					s.scheduleSaveLocked()
				}
				return
			}
		}
		return
	}

	if !s.images[idx].Seen {
		s.images[idx].Seen = true
		s.seenCount++
		s.scheduleSaveLocked()
	}
}

// Remove deletes an image from the store by its ID. It also deletes physical files asynchronously.
func (s *ImageStore) Remove(id string) (provider.Image, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	var img provider.Image
	for i, item := range s.images {
		if item.ID == id {
			idx = i
			img = item
			break
		}
	}

	img = s.images[idx]
	if img.Seen {
		s.seenCount--
	}

	// Remove from slice
	s.images = append(s.images[:idx], s.images[idx+1:]...)
	delete(s.idSet, id)
	if img.FilePath != "" {
		delete(s.pathSet, img.FilePath)
	}

	// Rebuild pathSet indices for remaining (O(N) but Remove is rare)
	for i := idx; i < len(s.images); i++ {
		if s.images[i].FilePath != "" {
			s.pathSet[s.images[i].FilePath] = i
		}
	}

	s.rebuildBucketsLocked() // Ensure buckets are updated after removal
	s.avoidSet[id] = true
	s.scheduleSaveLocked()

	if s.fm != nil {
		go func() { _ = s.fm.DeepDelete(id) }()
	}

	return img, true
}

// SeenCount returns the number of images that have been marked as seen.
func (s *ImageStore) SeenCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seenCount
}

// Clear resets the store to an empty state, but preserves the avoid set (blocklist).
func (s *ImageStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.images = make([]provider.Image, 0)
	s.idSet = make(map[string]bool)
	s.pathSet = make(map[string]int)
	s.resolutionBuckets = make(map[string][]string)
	s.seenCount = 0
	s.scheduleSaveLocked()
}

// ResetFavorites clears the IsFavorited flag on all images in the store.
func (s *ImageStore) ResetFavorites() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.images {
		s.images[i].IsFavorited = false
	}
	s.scheduleSaveLocked()
}

func (s *ImageStore) Wipe() {
	s.Clear()
	s.mu.Lock()
	s.avoidSet = make(map[string]bool)
	s.mu.Unlock()
	if s.fm != nil {
		go s.fm.CleanupOrphans(map[string]bool{})
	}
}

func (s *ImageStore) GetKnownIDs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	known := make(map[string]bool, len(s.idSet))
	for k, v := range s.idSet {
		known[k] = v
	}
	return known
}

// RemoveByQueryID removes all images associated with a specific provider query ID.
func (s *ImageStore) RemoveByQueryID(queryID string) {
	s.mu.Lock()
	var remaining []provider.Image
	var toDelete []provider.Image
	newSeenCount := 0
	for _, img := range s.images {
		if img.SourceQueryID == queryID {
			toDelete = append(toDelete, img)
			delete(s.idSet, img.ID)
			log.Printf("[RemoveByQueryID] Matched %s (Source: %s). Queueing for deletion.", img.ID, img.SourceQueryID)
			if img.FilePath != "" {
				delete(s.pathSet, img.FilePath)
			}
		} else {
			remaining = append(remaining, img)
			if img.Seen {
				newSeenCount++
			}
		}
	}
	s.images = remaining
	s.seenCount = newSeenCount

	// Re-index pathSet
	s.pathSet = make(map[string]int)
	for i, img := range s.images {
		if img.FilePath != "" {
			s.pathSet[img.FilePath] = i
		}
	}
	s.rebuildBucketsLocked() // Ensure buckets are updated after batch removal

	s.scheduleSaveLocked()
	s.mu.Unlock()

	if s.fm != nil {
		ids := make([]string, len(toDelete))
		for i, img := range toDelete {
			ids[i] = img.ID
		}
		go func(targetIDs []string) { _ = s.fm.DeepDeleteBatch(targetIDs) }(ids)
	}
}

func (s *ImageStore) List() []provider.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]provider.Image, len(s.images))
	copy(res, s.images)
	return res
}

func (s *ImageStore) LoadAvoidSet(avoidSet map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.avoidSet = make(map[string]bool)
	for k, v := range avoidSet {
		s.avoidSet[k] = v
	}
}

func (s *ImageStore) LoadCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cachePath == "" {
		return nil
	}

	file, err := os.Open(s.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&s.images); err != nil {
		return err
	}

	// Namespacing Migration: Upgrade legacy IDs to namespaced format
	migrationOccurred := false
	for i, img := range s.images {
		// Rule: Only online providers had raw numeric/string IDs.
		// Skip Favorites (already namespaced via filenames) and already-namespaced IDs.
		if img.Provider != "" && img.Provider != "Favorites" {
			prefix := img.Provider + "_"
			if !strings.HasPrefix(img.ID, prefix) {
				oldID := img.ID
				newID := prefix + oldID
				log.Printf("Migration: Upgrading legacy image ID %s -> %s for provider %s", oldID, newID, img.Provider)

				// 1. Rename files on disk (Master and Derivatives)
				if s.fm != nil {
					if err := s.fm.RenameAllAssets(oldID, newID); err != nil {
						log.Printf("Migration: Failed to rename assets for %s: %v", oldID, err)
					}
				}

				// 2. Update Image Metadata
				s.images[i].ID = newID

				// 3. Update Derivative Paths
				if img.DerivativePaths != nil {
					newPaths := make(map[string]string)
					for res, oldPath := range img.DerivativePaths {
						// Derivative paths include the ID in the filename
						newPath := strings.Replace(oldPath, oldID, newID, 1)
						newPaths[res] = newPath
					}
					s.images[i].DerivativePaths = newPaths
				}

				// 4. Update FilePath
				if img.FilePath != "" {
					s.images[i].FilePath = strings.Replace(img.FilePath, oldID, newID, 1)
				}

				migrationOccurred = true
			}
		}
	}

	if migrationOccurred {
		log.Printf("Migration: Namespacing upgrade complete. Saving updated cache.")
		// No lock needed as we're inside LoadCache which holds mu.Lock()
		s.scheduleSaveLocked()
	}

	s.idSet = make(map[string]bool)
	s.pathSet = make(map[string]int)
	s.resolutionBuckets = make(map[string][]string)
	s.seenCount = 0
	for i, img := range s.images {
		s.idSet[img.ID] = true
		if img.FilePath != "" {
			s.pathSet[img.FilePath] = i
		}
		if img.Seen {
			s.seenCount++
		}
		// Populate buckets from cache
		for res := range img.DerivativePaths {
			s.resolutionBuckets[res] = append(s.resolutionBuckets[res], img.ID)
		}
	}
	return nil
}

func (s *ImageStore) SaveCache() {
	s.mu.RLock()
	snapshot := make([]provider.Image, len(s.images))
	for i, img := range s.images {
		// Deep copy the struct to avoid shared map references during JSON encoding
		snapshot[i] = img
		if img.ProcessingFlags != nil {
			flags := make(map[string]bool)
			for k, v := range img.ProcessingFlags {
				flags[k] = v
			}
			snapshot[i].ProcessingFlags = flags
		}
		if img.DerivativePaths != nil {
			paths := make(map[string]string)
			for k, v := range img.DerivativePaths {
				paths[k] = v
			}
			snapshot[i].DerivativePaths = paths
		}
	}
	s.mu.RUnlock()

	s.saveCacheInternal(snapshot)
}

func (s *ImageStore) saveCacheInternal(images []provider.Image) {
	if s.saveFunc != nil {
		s.saveFunc()
	}

	if s.cachePath == "" {
		return
	}

	tmp := s.cachePath + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		log.Printf("Store: Failed to save cache: %v", err)
		return
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(images); err != nil {
		file.Close()
		log.Printf("Store: Failed to encode cache: %v", err)
		return
	}
	file.Close()

	if err := os.Rename(tmp, s.cachePath); err != nil {
		log.Printf("Store: Failed to rename cache: %v", err)
	}
}

// GetIDsForResolution returns a list of IDs compatible with the given resolution.
func (s *ImageStore) GetIDsForResolution(resolution string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, exists := s.resolutionBuckets[resolution]
	if !exists {
		return nil
	}

	res := make([]string, len(ids))
	copy(res, ids)
	return res
}

// GetBucketSize returns the number of images available for a specific resolution.
func (s *ImageStore) GetBucketSize(resolution string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.resolutionBuckets[resolution])
}

// ImageSyncAction defines the cleanup action required for an image.
type ImageSyncAction int

const (
	ImageActionKeep       ImageSyncAction = iota
	ImageActionDelete                     // Prune: Delete everything (Master + Derivatives)
	ImageActionInvalidate                 // Refresh: Delete only derivatives (Keep Master)
)

func (s *ImageStore) Sync(limit int, targetFlags map[string]bool, activeQueryIDs map[string]bool) {
	if s.fm == nil {
		return
	}

	if limit <= 0 {
		limit = 50
	}

	s.mu.RLock()
	candidates := make([]provider.Image, len(s.images))
	copy(candidates, s.images)
	s.mu.RUnlock()

	badIDs := make(map[string]ImageSyncAction)

	// 1. Determine actions for all candidates
	for _, img := range candidates {
		action := s.determineSyncAction(img, activeQueryIDs, targetFlags)
		if action != ImageActionKeep {
			badIDs[img.ID] = action
		}
	}

	s.mu.Lock()
	var finalImages []provider.Image
	var idsToDelete []string
	var idsToInvalidate []string

	// 2. Apply Actions (Delete/Invalidate)
	for _, img := range s.images {
		action, exists := badIDs[img.ID]
		if !exists || action == ImageActionKeep {
			finalImages = append(finalImages, img)
			continue
		}

		if action == ImageActionInvalidate {
			// Invalidate: Keep Master, Reset Flags/Paths
			img.DerivativePaths = make(map[string]string)
			img.ProcessingFlags = make(map[string]bool)
			for k, v := range targetFlags {
				img.ProcessingFlags[k] = v
			}
			finalImages = append(finalImages, img)
			idsToInvalidate = append(idsToInvalidate, img.ID)
		} else {
			// ActionDelete
			idsToDelete = append(idsToDelete, img.ID)
			delete(s.idSet, img.ID)
		}
	}

	// 3. Prune Limit
	if len(finalImages) > limit {
		excess := len(finalImages) - limit
		toPrune := finalImages[:excess]
		finalImages = finalImages[excess:]
		for _, img := range toPrune {
			idsToDelete = append(idsToDelete, img.ID)
			delete(s.idSet, img.ID)
		}
	}

	// 4. Update State
	s.images = finalImages
	s.rebuildInternalStateLocked()
	s.scheduleSaveLocked()
	s.mu.Unlock()

	// 5. Async Cleanup
	log.Printf("[Sync] Completed. Final Count: %d. Deleting: %d. Invalidating: %d.", len(finalImages), len(idsToDelete), len(idsToInvalidate))
	if len(idsToDelete) > 0 {
		log.Printf("[Sync] Deleting IDs: %v", idsToDelete)
	}
	s.performAsyncCleanup(idsToDelete, idsToInvalidate)
}

// determineSyncAction decides what to do with an image during sync.
func (s *ImageStore) determineSyncAction(img provider.Image, activeQueryIDs map[string]bool, targetFlags map[string]bool) ImageSyncAction {
	// Strict Mode Check
	if activeQueryIDs != nil {
		isActive := img.SourceQueryID != "" && activeQueryIDs[img.SourceQueryID]
		isOrphan := img.SourceQueryID == ""
		if isOrphan || !isActive {
			log.Printf("[Sync] Marking %s for deletion. Orphan: %v, ActiveSource: %v (SourceID: '%s')", img.ID, isOrphan, isActive, img.SourceQueryID)
			return ImageActionDelete
		}
	}

	// AvoidSet Check
	if s.avoidSet[img.ID] {
		return ImageActionDelete
	}

	// Master File Check
	if !s.masterFileExists(img.ID) {
		return ImageActionDelete
	}

	// Flag Mismatch Check
	if len(targetFlags) > 0 {
		if !s.flagsMatch(img, targetFlags) {
			return ImageActionInvalidate
		}
	}

	return ImageActionKeep
}

func (s *ImageStore) masterFileExists(id string) bool {
	statFunc := s.getStatFunc()

	// Check JPG
	masterPath, err := s.fm.GetMasterPath(id, ".jpg")
	if err == nil {
		if _, err := statFunc(masterPath); err == nil {
			return true
		}
	}

	// Check JPEG (Critical for Pexels which uses .jpeg)
	masterPath, err = s.fm.GetMasterPath(id, ".jpeg")
	if err == nil {
		if _, err := statFunc(masterPath); err == nil {
			return true
		}
	}

	// Check PNG
	masterPath, err = s.fm.GetMasterPath(id, ".png")
	if err == nil {
		if _, err := statFunc(masterPath); err == nil {
			return true
		}
	}

	return false
}

func (s *ImageStore) flagsMatch(img provider.Image, target map[string]bool) bool {
	if len(img.ProcessingFlags) != len(target) {
		return false
	}
	for k, v := range target {
		if img.ProcessingFlags[k] != v {
			return false
		}
	}
	return true
}

func (s *ImageStore) rebuildInternalStateLocked() {
	s.idSet = make(map[string]bool)
	s.pathSet = make(map[string]int)
	s.resolutionBuckets = make(map[string][]string)
	s.seenCount = 0
	for i, img := range s.images {
		s.idSet[img.ID] = true
		if img.FilePath != "" {
			s.pathSet[img.FilePath] = i
		}
		if img.Seen {
			s.seenCount++
		}
		for res := range img.DerivativePaths {
			s.resolutionBuckets[res] = append(s.resolutionBuckets[res], img.ID)
		}
	}
}

func (s *ImageStore) performAsyncCleanup(idsToDelete, idsToInvalidate []string) {
	if len(idsToDelete) > 0 {
		go func(targetIDs []string) { _ = s.fm.DeepDeleteBatch(targetIDs) }(idsToDelete)
	}
	if len(idsToInvalidate) > 0 {
		go func(ids []string) {
			for _, id := range ids {
				_ = s.fm.DeleteDerivatives(id)
			}
		}(idsToInvalidate)
	}
}
