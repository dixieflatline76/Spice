package wallpaper

import (
	"sync"

	"github.com/dixieflatline76/Spice/pkg/provider"
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
}

// NewImageStore creates a new ImageStore.
func NewImageStore() *ImageStore {
	return &ImageStore{
		images:   make([]provider.Image, 0),
		idSet:    make(map[string]bool),
		seen:     make(map[string]bool),
		avoidSet: make(map[string]bool),
	}
}

// Add adds a new image to the store if it hasn't been seen or avoided.
// Returns true if the image was added.
// It takes a Write Lock.
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

// Clear resets the store.
func (s *ImageStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.images = []provider.Image{}
	s.idSet = make(map[string]bool)
	s.seen = make(map[string]bool)
	s.avoidSet = make(map[string]bool)
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
