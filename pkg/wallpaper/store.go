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

	debounceDuration time.Duration
}

func NewImageStore() *ImageStore {
	return &ImageStore{
		images:           make([]provider.Image, 0),
		idSet:            make(map[string]bool),
		pathSet:          make(map[string]int),
		avoidSet:         make(map[string]bool),
		asyncSave:        true,
		debounceDuration: 2 * time.Second,
	}
}

func (s *ImageStore) SetDebounceDuration(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debounceDuration = d
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
			s.scheduleSaveLocked()
			return true
		}
	}
	return false
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
	s.scheduleSaveLocked()
	return true
}

func (s *ImageStore) Get(index int) (provider.Image, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if index >= 0 && index < len(s.images) {
		return s.images[index], true
	}
	return provider.Image{}, false
}

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

	s.avoidSet[id] = true
	s.scheduleSaveLocked()

	if s.fm != nil {
		go func() { _ = s.fm.DeepDelete(id) }()
	}

	return img, true
}

func (s *ImageStore) SeenCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seenCount
}

func (s *ImageStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.images = make([]provider.Image, 0)
	s.idSet = make(map[string]bool)
	s.pathSet = make(map[string]int)
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
	s.seenCount = 0
	for i, img := range s.images {
		s.idSet[img.ID] = true
		if img.FilePath != "" {
			s.pathSet[img.FilePath] = i
		}
		if img.Seen {
			s.seenCount++
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

	badIDs := make(map[string]bool)
	for _, img := range candidates {
		if activeQueryIDs != nil && img.SourceQueryID != "" && !activeQueryIDs[img.SourceQueryID] {
			badIDs[img.ID] = true
			continue
		}

		masterPath, err := s.fm.GetMasterPath(img.ID, ".jpg")
		if err != nil {
			badIDs[img.ID] = true
			continue
		}
		if _, err := os.Stat(masterPath); os.IsNotExist(err) {
			masterPath, err = s.fm.GetMasterPath(img.ID, ".png")
			if err != nil {
				badIDs[img.ID] = true
				continue
			}
			if _, err := os.Stat(masterPath); os.IsNotExist(err) {
				badIDs[img.ID] = true
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
				badIDs[img.ID] = true
			}
		}
	}

	s.mu.Lock()
	var finalImages []provider.Image
	var idsToDelete []string
	for _, img := range s.images {
		if shouldDelete, exists := badIDs[img.ID]; exists && shouldDelete {
			if !img.IsFavorited {
				idsToDelete = append(idsToDelete, img.ID)
			}
			delete(s.idSet, img.ID)
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
				idsToDelete = append(idsToDelete, img.ID)
			}
			delete(s.idSet, img.ID)
		}
	}

	s.images = finalImages
	s.idSet = make(map[string]bool)
	s.pathSet = make(map[string]int)
	s.seenCount = 0
	for i, img := range s.images {
		s.idSet[img.ID] = true
		if img.FilePath != "" {
			s.pathSet[img.FilePath] = i
		}
		if img.Seen {
			s.seenCount++
		}
	}

	s.scheduleSaveLocked()
	s.mu.Unlock()

	if len(idsToDelete) > 0 {
		go func(ids []string) {
			for _, id := range ids {
				_ = s.fm.DeepDelete(id)
			}
		}(idsToDelete)
	}
}

func (s *ImageStore) GetKnownIDs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make(map[string]bool)
	for k := range s.idSet {
		res[k] = true
	}
	return res
}
