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
	GetByID(id string) (provider.Image, bool)
	Remove(id string) (provider.Image, bool)
	MarkSeen(filePath string)
	SeenCount() int
	GetIDsForResolution(resolution string) []string
	GetBucketSize(resolution string) int
	GetUpdateChannel() <-chan struct{}
}

// MonitorState holds the persistence/cursor state for a single monitor.
type MonitorState struct {
	CurrentID        string
	History          []string
	RandomPos        int
	ShuffleIDs       []string // Each monitor tracks its own IDs for its resolution
	CurrentImage     provider.Image
	WaitingForImages bool
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

// NewMonitorController creates a new actor for managing a specific monitor's state.
func NewMonitorController(id int, m Monitor, store StoreInterface, fm *FileManager, os OS, cfg *Config, processor ImageProcessor) *MonitorController {
	return &MonitorController{
		ID:        id,
		Monitor:   m,
		Commands:  make(chan Command, 20), // Buffer slightly more to prevent blocking during bursts
		Store:     store,
		fm:        fm,
		os:        os,
		cfg:       cfg,
		processor: processor,
		State: &MonitorState{
			CurrentID:  "",
			History:    make([]string, 0),
			RandomPos:  0,
			ShuffleIDs: make([]string, 0),
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

// Stop sends a signal to terminate the actor loop.
func (mc *MonitorController) Stop() {
	if mc.cancel != nil {
		mc.cancel()
		mc.cancel = nil
	}
	mc.isRunning = false
}

// Run starts the monitor controller's actor loop.
func (mc *MonitorController) Run(ctx context.Context) {
	log.Debugf("[Monitor %d] Controller started", mc.ID)

	// Initial update channel
	updateCh := mc.Store.GetUpdateChannel()

	for {
		select {
		case <-ctx.Done():
			log.Debugf("[Monitor %d] Stopping controller", mc.ID)
			return
		case <-updateCh:
			// Refresh channel immediately for next event (broadcast pattern)
			updateCh = mc.Store.GetUpdateChannel()
			if mc.State.WaitingForImages {
				log.Debugf("[Monitor %d] Store updated while starving. Retrying next()...", mc.ID)
				mc.next()
			}
		case cmd := <-mc.Commands:
			mc.handleCommand(cmd)
		}
	}
}

func (mc *MonitorController) handleCommand(cmd Command) {
	log.Debugf("[Monitor %d] Actor received command %v (Pending: %d)", mc.ID, cmd, len(mc.Commands))
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
	width, height := mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy()
	resKey := fmt.Sprintf("%dx%d", width, height)

	// 1. Get/Refresh Bucket
	bucketIDs := mc.Store.GetIDsForResolution(resKey)

	// 2. Starvation/Cold Start Check
	// If bucket is zero OR below threshold, trigger fetch.
	// RequestFetch() handles debouncing and already-in-progress fetches.
	shouldFetch := false
	if len(bucketIDs) < BucketStarvationThreshold {
		shouldFetch = true
	} else if len(mc.State.ShuffleIDs) > 0 {
		// Cycle Progress: Trigger if we've cycled through 80% of our current shuffled list.
		if float64(mc.State.RandomPos) > float64(len(mc.State.ShuffleIDs))*PrcntSeenTillDownload {
			shouldFetch = true
		}
	}

	if shouldFetch {
		if mc.OnFetchRequest != nil {
			mc.OnFetchRequest()
		}
	}

	if len(bucketIDs) == 0 {
		log.Printf("[Monitor %d] No images found for resolution %s. Waiting for fetch...", mc.ID, resKey)
		mc.State.WaitingForImages = true
		return
	}
	mc.State.WaitingForImages = false

	// 3. Rebuild Shuffle if needed
	if len(mc.State.ShuffleIDs) != len(bucketIDs) {
		log.Printf("[Monitor %d] Bucket size changed (%d -> %d). Rebuilding shuffle.", mc.ID, len(mc.State.ShuffleIDs), len(bucketIDs))
		mc.rebuildShuffle(bucketIDs)
	}

	// 4. Pick Next
	if mc.State.RandomPos >= len(mc.State.ShuffleIDs) {
		mc.State.RandomPos = 0
		mc.rebuildShuffle(bucketIDs) // reshuffle when exhausted
	}

	nextID := mc.State.ShuffleIDs[mc.State.RandomPos]
	mc.State.RandomPos = (mc.State.RandomPos + 1) % len(mc.State.ShuffleIDs)

	if img, ok := mc.Store.GetByID(nextID); ok {
		mc.State.CurrentID = nextID
		mc.State.History = append(mc.State.History, nextID)
		mc.applyImage(img)
	}
}

func (mc *MonitorController) rebuildShuffle(ids []string) {
	shuffled := make([]string, len(ids))
	copy(shuffled, ids)

	if mc.cfg != nil && !mc.cfg.GetImgShuffle() {
		// Sequential - no shuffle needed, already copied in order
	} else {
		// Random
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
	}

	mc.State.ShuffleIDs = shuffled
	mc.State.RandomPos = 0
}

func (mc *MonitorController) prev() {
	if len(mc.State.History) <= 1 {
		return // Nothing to go back to
	}
	// Pop current
	mc.State.History = mc.State.History[:len(mc.State.History)-1]
	// Current is now last element
	prevID := mc.State.History[len(mc.State.History)-1]
	mc.State.CurrentID = prevID

	if img, ok := mc.Store.GetByID(prevID); ok {
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
		log.Debugf("[Monitor %d] Triggering async UI refresh for %s", mc.ID, path)
		mc.OnWallpaperChanged(img, mc.ID)
	}
}
