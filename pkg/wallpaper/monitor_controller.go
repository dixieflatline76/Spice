package wallpaper

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util/log"
)

// Command constants for the Actor loop
type Command int

const (
	CmdNext Command = iota
	CmdPrev
	CmdDelete
	CmdBlock
	CmdFavorite
)

// StoreInterface defines the subset of ImageStore methods needed by the controller.
type StoreInterface interface {
	Count() int
	Get(index int) (provider.Image, bool)
	Remove(id string) (provider.Image, bool)
	MarkSeen(filePath string)
	SeenCount() int
}

// MonitorState holds the persistence/cursor state for a single monitor.
type MonitorState struct {
	CurrentIndex int
	History      []int
	RandomPos    int
	ShuffleOrder []int // Each monitor technically tracks its own position in the global list
	CurrentImage provider.Image
}

// MonitorController is an Actor that manages one specific monitor.
// It receives commands via a channel and processes them sequentially.
type MonitorController struct {
	ID                 int
	Monitor            Monitor
	Commands           chan Command
	State              *MonitorState
	Store              StoreInterface
	fm                 *FileManager
	os                 OS
	cfg                *Config
	processor          ImageProcessor
	cancel             context.CancelFunc
	isRunning          bool
	OnWallpaperChanged func(img provider.Image, monitorID int)
	OnFavoriteRequest  func(img provider.Image)
	OnFetchRequest     func()
}

// NewMonitorController creates a new actor.
func NewMonitorController(id int, m Monitor, store StoreInterface, fm *FileManager, os OS, cfg *Config, processor ImageProcessor) *MonitorController {
	return &MonitorController{
		ID:        id,
		Monitor:   m,
		Commands:  make(chan Command, 10), // Buffer slightly to prevent blocking
		Store:     store,
		fm:        fm,
		os:        os,
		cfg:       cfg,
		processor: processor,
		State: &MonitorState{
			CurrentIndex: -1,
			History:      make([]int, 0),
			RandomPos:    0,
			ShuffleOrder: make([]int, 0),
		},
	}
}

// Start launches the actor loop.
func (mc *MonitorController) Start() {
	if mc.isRunning {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	mc.cancel = cancel
	mc.isRunning = true
	go mc.Run(ctx)
}

// Stop terminates the actor loop.
func (mc *MonitorController) Stop() {
	if mc.cancel != nil {
		mc.cancel()
		mc.cancel = nil
	}
	mc.isRunning = false
}

// Run is the main loop. It should be run in a goroutine.
func (mc *MonitorController) Run(ctx context.Context) {
	log.Debugf("[Monitor %d] Controller started", mc.ID)

	for {
		select {
		case <-ctx.Done():
			log.Debugf("[Monitor %d] Stopping controller", mc.ID)
			return
		case cmd := <-mc.Commands:
			mc.handleCommand(cmd)
		}
	}
}

func (mc *MonitorController) handleCommand(cmd Command) {
	log.Printf("[DEBUG] [Monitor %d] Actor handling command: %v", mc.ID, cmd)
	switch cmd {
	case CmdNext:
		mc.next()
	case CmdPrev:
		mc.prev()
	case CmdDelete:
		mc.deleteCurrent()
	case CmdFavorite:
		mc.toggleFavorite()
	}
}

func (mc *MonitorController) next() {
	count := mc.Store.Count()
	if count == 0 {
		log.Printf("[WARN] [Monitor %d] No images in store to advance to.", mc.ID)
		return
	}

	// Lazy Init Shuffle Order if empty or mismatched
	if len(mc.State.ShuffleOrder) != count {
		mc.rebuildShuffle(count)
	}

	// Try up to 50 times to find a compatible image for this monitor
	// to avoid "fallback" (incorrect aspect ratio) behavior.
	const maxAttempts = 50
	resKey := fmt.Sprintf("%dx%d", mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy())

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Advance Cursor
		// Check bounds just in case
		if mc.State.RandomPos >= len(mc.State.ShuffleOrder) {
			mc.State.RandomPos = 0
			mc.rebuildShuffle(count) // reshuffle when exhausted
		}

		nextIdx := mc.State.ShuffleOrder[mc.State.RandomPos]
		mc.State.RandomPos = (mc.State.RandomPos + 1) % len(mc.State.ShuffleOrder)

		// 2. Fetch Image
		if img, ok := mc.Store.Get(nextIdx); ok {
			// CHECK FOR COMPATIBILITY
			// If this image has derivatives logic (SmartFit on), verify we haveOUR resolution.
			// If DerivativePaths is empty, it might be a legacy image or SmartFit off, so we accept it.
			if len(img.DerivativePaths) > 0 {
				if _, hasRes := img.DerivativePaths[resKey]; !hasRes {
					// Skip this image, it wasn't generated for us (likely incompatible aspect)
					// Unless it's the LAST attempt, then we might accept it as fallback?
					// No, better to keep searching.
					// If we fail 50 times, we'll fall through and likely display the last one or exit.
					continue
				}
			}

			// Valid Match Found
			mc.State.CurrentIndex = nextIdx
			mc.State.History = append(mc.State.History, nextIdx) // Add to history
			mc.applyImage(img)
			return
		}
	}

	log.Printf("[WARN] [Monitor %d] Failed to find compatible image after %d attempts. Using fallback.", mc.ID, maxAttempts)
	// Fallback: Just use the current cursor pos (even if incompatible) to show SOMETHING
	// We rewind RandomPos by 1 to reuse the last picked index
	if mc.State.RandomPos > 0 {
		mc.State.RandomPos--
	}
	idx := mc.State.ShuffleOrder[mc.State.RandomPos]
	// Advance for next time
	mc.State.RandomPos = (mc.State.RandomPos + 1) % len(mc.State.ShuffleOrder)

	if img, ok := mc.Store.Get(idx); ok {
		mc.State.CurrentIndex = idx
		mc.State.History = append(mc.State.History, idx)
		mc.applyImage(img)
	}

	// Usage-Based Fetch Trigger (Restored)
	// Check if we have seen enough images to warrant a background fetch.
	// We use 0.8 (80%) as the legacy threshold.
	total := mc.Store.Count()
	if total > 0 {
		seen := mc.Store.SeenCount()
		threshold := int(float64(total) * 0.8) // Match legacy PrcntSeenTillDownload
		if seen >= threshold {
			if mc.OnFetchRequest != nil {
				mc.OnFetchRequest()
			}
		}
	}
}

func (mc *MonitorController) prev() {
	if len(mc.State.History) <= 1 {
		return // Nothing to go back to
	}
	// Pop current
	mc.State.History = mc.State.History[:len(mc.State.History)-1]
	// Current is now last element
	prevIdx := mc.State.History[len(mc.State.History)-1]
	mc.State.CurrentIndex = prevIdx

	if img, ok := mc.Store.Get(prevIdx); ok {
		mc.applyImage(img)
	}
}

func (mc *MonitorController) deleteCurrent() {
	img := mc.State.CurrentImage
	if img.ID == "" {
		return
	}
	log.Printf("[Monitor %d] Deleting image %s", mc.ID, img.ID)
	// 1. Mark as blocked/deleted in store
	mc.Store.Remove(img.ID)
	// 2. Add to avoid set
	if mc.cfg != nil {
		mc.cfg.AddToAvoidSet(img.ID)
	}

	// 3. Move to next
	mc.next()
}

func (mc *MonitorController) toggleFavorite() {
	img := mc.State.CurrentImage
	if img.ID == "" {
		return
	}
	if mc.OnFavoriteRequest != nil {
		mc.OnFavoriteRequest(img)
	}
}

func (mc *MonitorController) applyImage(img provider.Image) {
	// Determine resolution-specific path
	path := img.FilePath
	if len(img.DerivativePaths) > 0 {
		// Try to find exact match for this monitor's resolution
		resKey := fmt.Sprintf("%dx%d", mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy())
		if p, ok := img.DerivativePaths[resKey]; ok {
			path = p
			log.Debugf("[Monitor %d] Found exact resolution match: %s", mc.ID, resKey)
		} else if p, ok := img.DerivativePaths["primary"]; ok {
			path = p
			log.Debugf("[Monitor %d] Using primary fallback path", mc.ID)
		}
	}

	if path == "" {
		log.Printf("[ERROR] [Monitor %d] Cannot apply image %s: Path is empty (Derivatives: %v)", mc.ID, img.ID, img.DerivativePaths)
		return
	}

	mc.State.CurrentImage = img
	mc.State.CurrentImage.FilePath = path // Ensure state reflects actual file used

	log.Printf("[Monitor %d] Setting wallpaper: %s", mc.ID, path)
	if err := mc.os.SetWallpaper(path, mc.ID); err != nil {
		log.Printf("[ERROR] [Monitor %d] Failed to set wallpaper: %v", mc.ID, err)
	}
	mc.Store.MarkSeen(path)

	if mc.OnWallpaperChanged != nil {
		mc.OnWallpaperChanged(img, mc.ID)
	}
}

func (mc *MonitorController) rebuildShuffle(count int) {
	if mc.cfg != nil && !mc.cfg.GetImgShuffle() {
		// Sequential
		mc.State.ShuffleOrder = make([]int, count)
		for i := 0; i < count; i++ {
			mc.State.ShuffleOrder[i] = i
		}
	} else {
		// Random
		mc.State.ShuffleOrder = rand.Perm(count)
	}
}
