package wallpaper

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
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

	// QueryActiveFunc checks if a given source query ID is still active in the configuration.
	QueryActiveFunc func(string) bool
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

// SetQueryActiveFunc sets the callback used to check if an image's source query is still active
func (s *ImageStore) SetQueryActiveFunc(fn func(string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.QueryActiveFunc = fn
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

// replace performs a full struct replacement for an existing image.
// This is unexported because full replacement is dangerous — callers who only
// need to change one field should use SetFavorited, SetTuningOptions, or ClearDerivatives.
// The only legitimate caller is stateManagerLoop (pipeline), which always has
// a fully-populated Image from ProcessImageJob.
func (s *ImageStore) replace(img provider.Image) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.images {
		if existing.ID == img.ID {
			// Diagnostic: Log when DerivativePaths are being cleared.
			// This is legitimate when files are missing (applyImage stale detection),
			// but suspicious if it happens during pipeline processing.
			if len(img.DerivativePaths) == 0 && len(existing.DerivativePaths) > 0 {
				log.Debugf("[Store] WARNING: replace() clearing DerivativePaths for %s (had %d paths)", img.ID, len(existing.DerivativePaths))
			}
			// Preserve user-set Tuning - the store is authoritative for user metadata.
			// The pipeline never writes Tuning, so existing store values always win.
			// The else branch ensures that if the user removed all tuning
			// while the pipeline was in-flight, stale tuning from MergeExistingMetadata
			// are not resurrected.
			if len(existing.Tuning) > 0 {
				img.Tuning = make(map[string]provider.TuningOptions, len(existing.Tuning))
				for k, v := range existing.Tuning {
					img.Tuning[k] = v
				}
			} else {
				img.Tuning = nil
			}
			// Incremental bucket update: remove old entries, add new ones
			s.removeFromBucketsLocked(existing.ID, existing.DerivativePaths)
			s.images[i] = img
			s.addToBucketsLocked(img.ID, img.DerivativePaths)
			s.scheduleSaveLocked()
			return true
		}
	}
	return false
}

// SetFavorited updates only the IsFavorited flag for an image.
func (s *ImageStore) SetFavorited(id string, favorited bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.images {
		if s.images[i].ID == id {
			s.images[i].IsFavorited = favorited
			s.scheduleSaveLocked()
			return true
		}
	}
	return false
}

// SetTuningOptions updates the tuning options for a specific resolution on an image.
// Pass an empty TuningOptions struct to clear the overrides.
func (s *ImageStore) SetTuningOptions(id string, resKey string, opts provider.TuningOptions) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.images {
		if s.images[i].ID == id {
			if s.images[i].Tuning == nil {
				s.images[i].Tuning = make(map[string]provider.TuningOptions)
			}
			// If it's the default/empty state, remove it from the map to clean up JSON
			defaultOpts := provider.TuningOptions{Anchor: provider.AnchorAuto}
			if opts == defaultOpts {
				delete(s.images[i].Tuning, resKey)
			} else {
				s.images[i].Tuning[resKey] = opts
			}
			s.scheduleSaveLocked()
			return true
		}
	}
	return false
}

// ClearDerivatives resets an image's DerivativePaths and ProcessingFlags.
// Used by monitor_controller when a derivative file is missing from disk.
func (s *ImageStore) ClearDerivatives(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.images {
		if s.images[i].ID == id {
			// Incremental bucket update: remove this image's entries
			s.removeFromBucketsLocked(id, s.images[i].DerivativePaths)
			s.images[i].DerivativePaths = make(map[string]string)
			s.images[i].ProcessingFlags = make(map[string]bool)
			s.scheduleSaveLocked()
			return true
		}
	}
	return false
}

// rebuildBucketsLocked re-scans ALL images and rebuilds resolution buckets from scratch.
// Use only for batch operations (e.g. RemoveByQueryID, Sync). For single-image
// mutations, prefer addToBucketsLocked/removeFromBucketsLocked.
// CALLER MUST HOLD s.mu.Lock().
func (s *ImageStore) rebuildBucketsLocked() {
	s.resolutionBuckets = make(map[string][]string)
	for _, img := range s.images {
		for res := range img.DerivativePaths {
			s.resolutionBuckets[res] = append(s.resolutionBuckets[res], img.ID)
		}
	}
}

// addToBucketsLocked adds a single image's derivative paths to the resolution buckets.
// CALLER MUST HOLD s.mu.Lock().
func (s *ImageStore) addToBucketsLocked(id string, derivativePaths map[string]string) {
	for res := range derivativePaths {
		s.resolutionBuckets[res] = append(s.resolutionBuckets[res], id)
	}
}

// removeFromBucketsLocked removes a single image's entries from the resolution buckets.
// CALLER MUST HOLD s.mu.Lock().
func (s *ImageStore) removeFromBucketsLocked(id string, derivativePaths map[string]string) {
	for res := range derivativePaths {
		ids := s.resolutionBuckets[res]
		for j, bucketID := range ids {
			if bucketID == id {
				s.resolutionBuckets[res] = append(ids[:j], ids[j+1:]...)
				break
			}
		}
		// Clean up empty bucket entries
		if len(s.resolutionBuckets[res]) == 0 {
			delete(s.resolutionBuckets, res)
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
		// Favorites Upsert: selectively update only the fields the Favorites
		// provider can authoritatively set, preserving all store-managed metadata
		// (DerivativePaths, ProcessingFlags, Tuning, Width, Height, etc.).
		//
		// Uses non-empty-wins for Attribution/ViewURL because AddFavorite is
		// async (queues a job), but RequestFetch fires immediately after — the
		// fetch can race with the metadata.json write, arriving with empty fields.
		if img.SourceQueryID == FavoritesQueryID {
			for i, existing := range s.images {
				if existing.ID == img.ID {
					// Provider-authoritative fields: always update
					s.images[i].Provider = img.Provider
					s.images[i].SourceQueryID = img.SourceQueryID
					s.images[i].IsFavorited = true

					// Download URL: always update (Favorites provider serves via local API)
					if img.Path != "" {
						s.images[i].Path = img.Path
					}

					// Non-empty-wins: preserve existing if incoming is empty (race protection)
					if img.Attribution != "" {
						s.images[i].Attribution = img.Attribution
					}
					if img.ViewURL != "" {
						s.images[i].ViewURL = img.ViewURL
					}

					// Seen: preserve if already seen
					if existing.Seen && !img.Seen {
						s.images[i].Seen = true
					}

					// FilePath: update pathSet if changed
					if img.FilePath != "" && img.FilePath != existing.FilePath {
						if existing.FilePath != "" {
							delete(s.pathSet, existing.FilePath)
						}
						s.images[i].FilePath = img.FilePath
						s.pathSet[img.FilePath] = i
					}

					s.scheduleSaveLocked()
					return true
				}
			}
		}
		return false
	}

	// Active Query Check: Reject images from queries disabled mid-download
	if s.QueryActiveFunc != nil && img.SourceQueryID != "" {
		if !s.QueryActiveFunc(img.SourceQueryID) {
			log.Debugf("Store: Rejecting new image %s from inactive query %s", img.ID, img.SourceQueryID)
			return false
		}
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

// Exists checks if an image ID is already in the store.
func (s *ImageStore) Exists(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.idSet[id]
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
	for i, item := range s.images {
		if item.ID == id {
			idx = i
			break
		}
	}

	if idx == -1 {
		return provider.Image{}, false
	}

	img := s.images[idx]
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

	s.removeFromBucketsLocked(id, img.DerivativePaths) // Incremental bucket removal
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
			log.Debugf("[RemoveByQueryID] Matched %s (Source: %s). Queueing for deletion.", img.ID, img.SourceQueryID)
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
	migrationOccurred := s.migrateLegacyIDsLocked()

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
		if img.Tuning != nil {
			tuning := make(map[string]provider.TuningOptions)
			for k, v := range img.Tuning {
				tuning[k] = v
			}
			snapshot[i].Tuning = tuning
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
	log.Debugf("[Sync] Completed. Final Count: %d. Deleting: %d. Invalidating: %d.", len(finalImages), len(idsToDelete), len(idsToInvalidate))
	if len(idsToDelete) > 0 {
		log.Debugf("[Sync] Deleting IDs: %v", idsToDelete)
	}
	s.performAsyncCleanup(idsToDelete, idsToInvalidate)
}

// determineSyncAction decides what to do with an image during sync.
func (s *ImageStore) determineSyncAction(img provider.Image, activeQueryIDs map[string]bool, targetFlags map[string]bool) ImageSyncAction {
	isProtected := img.IsFavorited || img.Provider == "Favorites" || strings.Contains(img.ID, "_favorite_images_")

	// Strict Mode Check
	if activeQueryIDs != nil && !isProtected {
		isActive := img.SourceQueryID != "" && activeQueryIDs[img.SourceQueryID]
		isOrphan := img.SourceQueryID == ""
		if isOrphan || !isActive {
			log.Debugf("[Sync] Marking %s for deletion. Orphan: %v, ActiveSource: %v (SourceID: '%s')", img.ID, isOrphan, isActive, img.SourceQueryID)
			return ImageActionDelete
		}
	}

	// AvoidSet Check
	if s.avoidSet[img.ID] && !isProtected {
		return ImageActionDelete
	}

	// Master File Check
	if !s.masterFileExists(img.ID) {
		// If it's a known incompatible image, keep it in the DB (as a zombie)
		// so we don't forget its processing flags and redownload it over and over!
		if s.hasIncompatibleFlags(img) {
			return ImageActionKeep
		}
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

func (s *ImageStore) hasIncompatibleFlags(img provider.Image) bool {
	for k, v := range img.ProcessingFlags {
		if v && strings.HasPrefix(k, "incompatible:") {
			return true
		}
	}
	return false
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
	// Compare only the processing-mode keys from `target`.
	// img.ProcessingFlags may also contain "incompatible:<WxH>" metadata tags
	// that are NOT part of the processing mode, so a strict length comparison
	// would always fail for images that have any incompatibility tags.
	for k, v := range target {
		if img.ProcessingFlags[k] != v {
			return false
		}
	}
	// Reverse check: ensure the image doesn't have processing-mode keys
	// that aren't in target (guards against future flag additions).
	for k, v := range img.ProcessingFlags {
		if strings.HasPrefix(k, "incompatible:") {
			continue // skip metadata tags
		}
		if target[k] != v {
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

// migrateLegacyIDsLocked upgrades legacy image IDs to the namespaced format (Provider_ID).
// CALLER MUST HOLD s.mu.Lock()
func (s *ImageStore) migrateLegacyIDsLocked() bool {
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
	return migrationOccurred
}
