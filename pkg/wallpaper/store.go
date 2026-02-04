package wallpaper

import (
	"encoding/json"
	"os"
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

func (s *ImageStore) Wipe() {
	s.Clear()
	s.mu.Lock()
	s.avoidSet = make(map[string]bool)
	s.mu.Unlock()
	if s.fm != nil {
		go s.fm.CleanupOrphans(map[string]bool{})
	}
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
		for _, img := range toDelete {
			go func(id string) { _ = s.fm.DeepDelete(id) }(img.ID)
		}
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

	// SyncAction defines the cleanup action required for an image.
	type SyncAction int
	const (
		ActionKeep       SyncAction = iota
		ActionDelete                // Prune: Delete everything (Master + Derivatives)
		ActionInvalidate            // Refresh: Delete only derivatives (Keep Master)
	)

	badIDs := make(map[string]SyncAction)

	for _, img := range candidates {
		// Strict Sync: If activeQueryIDs is provided (Strict Mode),
		// we filter out images that:
		// 1. Have a SourceQueryID that is NOT in the active set.
		// 2. Have NO SourceQueryID (Orphan/Legacy), as they cannot be verified against the active set.
		// Exception: Favorites are protected from deletion later (check below),
		// but they are still removed from the *active rotation* here unless their SourceQuery is active.
		isOrphan := img.SourceQueryID == ""
		isActive := false
		if img.SourceQueryID != "" && activeQueryIDs != nil {
			isActive = activeQueryIDs[img.SourceQueryID]
		}

		if activeQueryIDs != nil {
			if isOrphan || !isActive {
				badIDs[img.ID] = ActionDelete
				continue
			}
		}

		if s.avoidSet[img.ID] {
			badIDs[img.ID] = ActionDelete
			continue
		}

		masterPath, err := s.fm.GetMasterPath(img.ID, ".jpg")
		if err != nil {
			badIDs[img.ID] = ActionDelete
			continue
		}
		statFunc := s.getStatFunc()
		if _, err := statFunc(masterPath); os.IsNotExist(err) {
			masterPath, err = s.fm.GetMasterPath(img.ID, ".png")
			if err != nil {
				badIDs[img.ID] = ActionDelete
				continue
			}
			if _, err := statFunc(masterPath); os.IsNotExist(err) {
				badIDs[img.ID] = ActionDelete
				continue
			}
		}

		// C. Flag Mismatch
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
				// Mismatch means the derivative is stale, but the image itself (Master) is valid.
				// We should only delete the derivatives so the downloader can re-process the Master.
				badIDs[img.ID] = ActionInvalidate
			}
		}
	}

	s.mu.Lock()
	var finalImages []provider.Image
	var idsToDelete []string
	var idsToInvalidate []string

	for _, img := range s.images {
		if action, exists := badIDs[img.ID]; exists && action != ActionKeep {
			if action == ActionInvalidate {
				// Mismatch means the derivative is stale.
				// Clear paths and flags so it can be re-processed with new targets.
				img.DerivativePaths = make(map[string]string)
				img.ProcessingFlags = make(map[string]bool)
				for k, v := range targetFlags {
					img.ProcessingFlags[k] = v
				}
				finalImages = append(finalImages, img)
				idsToInvalidate = append(idsToInvalidate, img.ID)
			} else {
				// ActionDelete
				if !img.IsFavorited {
					idsToDelete = append(idsToDelete, img.ID)
				} else {
					// Fallback for Favorites: Just invalidate if they were supposed to be deleted
					img.DerivativePaths = make(map[string]string)
					img.ProcessingFlags = make(map[string]bool)
					for k, v := range targetFlags {
						img.ProcessingFlags[k] = v
					}
					finalImages = append(finalImages, img)
					idsToInvalidate = append(idsToInvalidate, img.ID)
				}
				delete(s.idSet, img.ID)
			}
		} else {
			finalImages = append(finalImages, img)
		}
	}

	if len(finalImages) > limit {
		excess := len(finalImages) - limit
		toPrune := finalImages[:excess]
		finalImages = finalImages[excess:]
		for _, img := range toPrune {
			if !img.IsFavorited {
				idsToDelete = append(idsToDelete, img.ID) // Pruning = Delete
			}
			delete(s.idSet, img.ID)
		}
	}

	s.images = finalImages
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
		// Rebuild buckets during sync
		for res := range img.DerivativePaths {
			s.resolutionBuckets[res] = append(s.resolutionBuckets[res], img.ID)
		}
	}

	s.scheduleSaveLocked()
	s.mu.Unlock()

	// Async Cleanup
	if len(idsToDelete) > 0 {
		go func(ids []string) {
			for _, id := range ids {
				// Race Condition Fix: Check if the ID has been resurrected (re-added) to the store
				// since the sync operation started. If so, DO NOT delete the files.
				if _, exists := s.GetByID(id); exists {
					log.Printf("[Store] Skipping deletion of resurrected ID: %s", id)
					continue
				}
				_ = s.fm.DeepDelete(id)
			}
		}(idsToDelete)
	}
	if len(idsToInvalidate) > 0 {
		go func(ids []string) {
			for _, id := range ids {
				_ = s.fm.DeleteDerivatives(id)
			}
		}(idsToInvalidate)
	}
}

// GetKnownIDs returns a map of all image IDs currently in the store.
func (s *ImageStore) GetKnownIDs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make(map[string]bool)
	for k := range s.idSet {
		res[k] = true
	}
	return res
}
