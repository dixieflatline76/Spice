package wallpaper

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// Command constants for the Actor loop
type Command int

const (
	CmdNext Command = iota
	CmdPrev
	CmdDelete
	CmdBlock
	CmdFavorite
	CmdUpdateShuffle
	CmdSyncState
	CmdPause
	CmdNextAuto

	// Tuning Commands
	CmdTuningStart
	CmdTuningEnd

	// Legacy Anchor commands
	CmdAnchorAuto Command = 200
	CmdAnchorTL   Command = 201
	CmdAnchorTC   Command = 202
	CmdAnchorTR   Command = 203
	CmdAnchorML   Command = 204
	CmdAnchorMC   Command = 205
	CmdAnchorMR   Command = 206
	CmdAnchorBL   Command = 207
	CmdAnchorBC   Command = 208
	CmdAnchorBR   Command = 209
)

// StoreInterface defines the subset of ImageStore methods needed by the controller.
type StoreInterface interface {
	Count() int
	Get(index int) (provider.Image, bool)
	GetByID(id string) (provider.Image, bool)
	Exists(id string) bool
	Remove(id string) (provider.Image, bool)
	SetFavorited(id string, favorited bool) bool
	SetTuningOptions(id string, resKey string, opts provider.TuningOptions) bool
	ClearDerivatives(id string) bool
	Add(img provider.Image) bool
	Clear()
	MarkSeen(filePath string)
	SeenCount() int
	GetIDsForResolution(resolution string) []string
	GetBucketSize(resolution string) int
	GetUpdateChannel() <-chan struct{}

	// Administrative and Batch Operations
	Sync(limit int, targetFlags map[string]bool, activeQueryIDs map[string]bool)
	GetKnownIDs() map[string]bool
	SetFileManager(fm *FileManager, cacheFile string)
	SetAsyncSave(enabled bool)
	SetDebounceDuration(d time.Duration)
	SetQueryActiveFunc(fn func(string) bool)
	LoadCache() error
	LoadAvoidSet(avoidSet map[string]bool)
	Wipe()
	RemoveByQueryID(queryID string)
	ResetFavorites()
	List() []provider.Image
	WaitForImages(ctx context.Context) error
}

// MonitorMenuItems holds the tray menu items for a specific monitor.
type MonitorMenuItems struct {
	ProviderMenuItem *fyne.MenuItem
	ArtistMenuItem   *fyne.MenuItem
	FavoriteMenuItem *fyne.MenuItem
	PauseMenuItem    *fyne.MenuItem
	ShuffleMenuItem  *fyne.MenuItem
}

// MonitorState holds the persistence/cursor state for a single monitor.
type MonitorState struct {
	CurrentID        string
	History          []string
	RandomPos        int
	ShuffleIDs       []string // Each monitor tracks its own IDs for its resolution
	CurrentImage     provider.Image
	WaitingForImages bool
	Paused           bool
	TuningInProgress bool
	ManualRecovery   bool // True if WaitingForImages was triggered by a manual request
}

// MonitorController is an Actor that manages one specific monitor.
// It receives commands via a channel and processes them sequentially.
type MonitorController struct {
	mu sync.RWMutex

	ID                 int
	Monitor            Monitor
	Commands           chan Command
	TuningChan         chan provider.TuningOptions
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
	pendingUpdate      bool // Flag to indicate Store content has changed
}

// NewMonitorController creates a new actor for managing a specific monitor's state.
func NewMonitorController(id int, m Monitor, store StoreInterface, fm *FileManager, os OS, cfg *Config, processor ImageProcessor) *MonitorController {
	paused := false
	if cfg != nil {
		paused = cfg.IsMonitorPaused(m.DevicePath)
	}

	return &MonitorController{
		ID:         id,
		Monitor:    m,
		Commands:   make(chan Command, 50),
		TuningChan: make(chan provider.TuningOptions, 50), // Buffer slightly more to prevent blocking during bursts
		Store:      store,
		fm:         fm,
		os:         os,
		cfg:        cfg,
		processor:  processor,
		State: &MonitorState{
			CurrentID:  "",
			History:    make([]string, 0),
			RandomPos:  0,
			ShuffleIDs: make([]string, 0),
			Paused:     paused,
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
			mc.pendingUpdate = true
			if mc.State.WaitingForImages {
				log.Debugf("[Monitor %d] Store updated while starving. Retrying next(manual=%v)...", mc.ID, mc.State.ManualRecovery)
				mc.next(mc.State.ManualRecovery)
			}
		case cmd := <-mc.Commands:
			mc.mu.Lock()
			mc.handleCommand(cmd)
			mc.mu.Unlock()
		case tuning := <-mc.TuningChan:
			mc.mu.Lock()
			mc.reprocessWithTuning(tuning)
			mc.mu.Unlock()
		}
	}
}

func (mc *MonitorController) handleCommand(cmd Command) {
	log.Debugf("[Monitor %d] Actor received command %v (Pending: %d)", mc.ID, cmd, len(mc.Commands))

	// Legacy Anchor commands (200-209)
	if cmd >= CmdAnchorAuto && cmd <= CmdAnchorBR {
		img := mc.State.CurrentImage
		if img.ID == "" {
			return
		}
		resKey := fmt.Sprintf("%dx%d", mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy())
		opts := img.GetTuning(resKey)
		opts.Anchor = provider.CropAnchor(cmd)
		mc.reprocessWithTuning(opts)
		return
	}

	switch cmd {
	case CmdNext:
		mc.next(true)
	case CmdNextAuto:
		mc.next(false)
	case CmdPrev:
		mc.prev()
	case CmdDelete:
		mc.deleteCurrent()
	case CmdFavorite:
		mc.toggleFavorite()
	case CmdUpdateShuffle:
		mc.updateShuffle()
	case CmdSyncState:
		mc.syncState()
	case CmdPause:
		mc.togglePause()
	case CmdTuningStart:
		mc.State.TuningInProgress = true
	case CmdTuningEnd:
		mc.State.TuningInProgress = false
	}
}

func (mc *MonitorController) togglePause() {
	mc.State.Paused = !mc.State.Paused
	if mc.cfg != nil {
		mc.cfg.SetMonitorPaused(mc.Monitor.DevicePath, mc.State.Paused)
	}
	log.Printf("[Monitor %d] Pause set to %v", mc.ID, mc.State.Paused)
	if mc.OnWallpaperChanged != nil {
		mc.OnWallpaperChanged(mc.State.CurrentImage, mc.ID)
	}
}

func (mc *MonitorController) next(manual bool) {
	if !manual && (mc.State.Paused || mc.State.TuningInProgress) {
		log.Debugf("[Monitor %d] Skipping automatic Next (Monitor is paused or tuning)", mc.ID)
		return
	}
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
		mc.State.ManualRecovery = manual
		return
	}
	mc.State.WaitingForImages = false
	mc.State.ManualRecovery = false

	// 3. Reconcile Shuffle with Bucket
	// Incremental strategy: when the pool grows, scatter new images into the
	// unplayed portion of the deck. This prevents provider clustering when
	// images arrive in same-provider bursts. Full rebuilds only happen on
	// deck exhaustion or pool shrinkage.
	if len(mc.State.ShuffleIDs) != len(bucketIDs) {
		if len(bucketIDs) > len(mc.State.ShuffleIDs) && len(mc.State.ShuffleIDs) > 0 {
			mc.growShuffle(bucketIDs)
		} else {
			// Pool shrank or was empty — full rebuild
			log.Debugf("[Monitor %d] Shuffle full rebuild (Bucket: %d, Current: %d).", mc.ID, len(bucketIDs), len(mc.State.ShuffleIDs))
			mc.rebuildShuffle(bucketIDs)
		}
	}
	mc.pendingUpdate = false // Always consume the pending update

	// 4. Pick Next (with Skip/Retry Logic for Blocked/Missing images)
	for attempts := 0; attempts < len(mc.State.ShuffleIDs); attempts++ {
		if mc.State.RandomPos >= len(mc.State.ShuffleIDs) {
			mc.State.RandomPos = 0
			mc.rebuildShuffle(bucketIDs) // reshuffle when exhausted
		}

		nextID := mc.State.ShuffleIDs[mc.State.RandomPos]
		mc.State.RandomPos = (mc.State.RandomPos + 1) % len(mc.State.ShuffleIDs)

		// Layer 2 Defense: Check AvoidSet (Block List)
		if mc.cfg != nil && mc.cfg.InAvoidSet(nextID) {
			log.Debugf("[Monitor %d] Skipping blocked image during rotation: %s", mc.ID, nextID)
			continue
		}

		if img, ok := mc.Store.GetByID(nextID); ok {
			mc.State.CurrentID = nextID
			mc.State.History = append(mc.State.History, nextID)

			// 5. Cap History (Resource Management)
			if len(mc.State.History) > 100 {
				mc.State.History = mc.State.History[1:]
			}

			mc.applyImage(img)
			return // Successfully found and applied an image
		} else {
			log.Debugf("[Monitor %d] Skipping image missing from store (likely recently deleted): %s", mc.ID, nextID)
		}
	}

	log.Printf("[Monitor %d] All images in shuffle list are blocked or missing. Waiting for fetch...", mc.ID)
	mc.State.WaitingForImages = true
	mc.State.ManualRecovery = manual
}

// rebuildShuffle performs a full shuffle of all IDs and resets position to 0.
// Used on: initial build, deck exhaustion, pool shrinkage.
func (mc *MonitorController) rebuildShuffle(ids []string) {
	shuffled := make([]string, len(ids))
	copy(shuffled, ids)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	mc.State.ShuffleIDs = shuffled
	mc.State.RandomPos = 0
}

// growShuffle incrementally inserts newly arrived images into the unplayed
// portion of the deck. This preserves the current playback position and
// prevents provider clustering when images arrive in same-provider bursts.
func (mc *MonitorController) growShuffle(bucketIDs []string) {
	// Build set of IDs already in the shuffle deck
	existing := make(map[string]bool, len(mc.State.ShuffleIDs))
	for _, id := range mc.State.ShuffleIDs {
		existing[id] = true
	}

	// Collect new IDs not yet in the deck
	var newIDs []string
	for _, id := range bucketIDs {
		if !existing[id] {
			newIDs = append(newIDs, id)
		}
	}

	if len(newIDs) == 0 {
		return
	}

	// Shuffle the new arrivals
	rand.Shuffle(len(newIDs), func(i, j int) {
		newIDs[i], newIDs[j] = newIDs[j], newIDs[i]
	})

	// Insert new IDs at random positions in the UNPLAYED portion of the deck
	// (everything from RandomPos onward). This scatters them among existing
	// unseen images instead of clustering them at the end.
	played := mc.State.ShuffleIDs[:mc.State.RandomPos]
	unplayed := mc.State.ShuffleIDs[mc.State.RandomPos:]

	// Merge: interleave new IDs into unplayed portion
	merged := make([]string, 0, len(unplayed)+len(newIDs))
	merged = append(merged, unplayed...)
	merged = append(merged, newIDs...)

	// Shuffle only the merged unplayed portion to scatter new arrivals
	rand.Shuffle(len(merged), func(i, j int) {
		merged[i], merged[j] = merged[j], merged[i]
	})

	// Reassemble: played portion stays intact, unplayed is now mixed
	mc.State.ShuffleIDs = append(played, merged...)

	log.Debugf("[Monitor %d] Incremental shuffle: inserted %d new images into unplayed deck (Pos: %d, Total: %d)",
		mc.ID, len(newIDs), mc.State.RandomPos, len(mc.State.ShuffleIDs))
}

func (mc *MonitorController) updateShuffle() {
	log.Debugf("[Monitor %d] Updating shuffle state...", mc.ID)
	width, height := mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy()
	resKey := fmt.Sprintf("%dx%d", width, height)

	// Get current active bucket
	bucketIDs := mc.Store.GetIDsForResolution(resKey)

	// Rebuild shuffle with new config state
	mc.rebuildShuffle(bucketIDs)

	// Note: We do NOT force a wallpaper change here (CmdNext) to avoid jarring the user.
	// The next automatic or manual change will pick from the new order.
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

	// BACKTRACKING FIX:
	// We must also step back in our shuffle list (RandomPos) so that the next "Next"
	// call returns us to where we were, rather than skipping ahead.
	// We use modulo arithmetic to handle wrapping safely, though typically
	// prev() implies we have history.
	// RandomPos points to the *next* item to be shown.
	// So if we go back, we decrement it.
	if len(mc.State.ShuffleIDs) > 0 {
		mc.State.RandomPos--
		if mc.State.RandomPos < 0 {
			mc.State.RandomPos = len(mc.State.ShuffleIDs) - 1
		}
	}

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
	mc.next(true)
}

func (mc *MonitorController) toggleFavorite() {
	img := mc.State.CurrentImage
	if img.ID == "" {
		return
	}
	if mc.OnFavoriteRequest != nil {
		// CRITICAL: Run outside mc.mu.Lock() scope via goroutine.
		// ToggleFavorite iterates all monitors and acquires mc.mu.RLock(),
		// which would deadlock if called synchronously (same goroutine holds mc.mu.Lock).
		go mc.OnFavoriteRequest(img)
	}
}

func (mc *MonitorController) syncState() {
	log.Debugf("[Monitor %d] Syncing state from store...", mc.ID)
	if mc.State.CurrentID == "" {
		return
	}

	if img, ok := mc.Store.GetByID(mc.State.CurrentID); ok {
		log.Debugf("[Monitor %d] Metadata updated for %s (Favorited: %v)", mc.ID, img.ID, img.IsFavorited)
		mc.State.CurrentImage = img
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

	// Check if file physically exists before calling OS to avoid generic errors
	if _, err := mc.os.Stat(path); os.IsNotExist(err) {
		log.Printf("[Monitor %d] ERROR: Wallpaper file missing: %s. Metadata is stale. Requesting refetch...", mc.ID, path)
		// Clear local metadata that is proven stale so it's not chosen again
		mc.Store.ClearDerivatives(img.ID)
		if mc.OnFetchRequest != nil {
			mc.OnFetchRequest()
		}
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

// reprocessWithTuning updates the tuning options on the current image, re-runs FitImage,
// and sets the resulting derivative as the wallpaper.
func (mc *MonitorController) reprocessWithTuning(opts provider.TuningOptions) {
	img := mc.State.CurrentImage
	if img.ID == "" {
		log.Printf("[Monitor %d] Cannot reprocess tuning: no current image", mc.ID)
		return
	}

	log.Printf("[Monitor %d] Reprocessing with tuning %v for image %s", mc.ID, opts, img.ID)

	// 1. Update tuning in image metadata for this monitor's resolution and persist
	resKey := fmt.Sprintf("%dx%d", mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy())
	mc.Store.SetTuningOptions(img.ID, resKey, opts)

	// Mirror the tuning change into the local image copy so that
	// mc.State.CurrentImage stays in sync with the store. Without this,
	// the tuning popup would show the stale (pre-change) tuning until
	// the user navigates away and back.
	if img.Tuning == nil {
		img.Tuning = make(map[string]provider.TuningOptions)
	}
	defaultOpts := provider.TuningOptions{Anchor: provider.AnchorAuto}
	if opts == defaultOpts {
		delete(img.Tuning, resKey)
	} else {
		img.Tuning[resKey] = opts
	}
	mc.State.CurrentImage = img

	// 2. Find master path
	ext := filepath.Ext(img.FilePath)
	if ext == "" {
		ext = ".jpg"
	}
	masterPath, err := mc.fm.GetMasterPath(img.ID, ext)
	if err != nil {
		log.Printf("[ERROR] [Monitor %d] Failed to get master path: %v", mc.ID, err)
		return
	}
	if _, err := os.Stat(masterPath); os.IsNotExist(err) {
		log.Printf("[ERROR] [Monitor %d] Master file missing: %s", mc.ID, masterPath)
		return
	}

	// 3. Open and decode master
	srcImg, err := imaging.Open(masterPath)
	if err != nil {
		log.Printf("[ERROR] [Monitor %d] Failed to open master: %v", mc.ID, err)
		return
	}

	// 4. Re-run FitImage with new tuning
	width, height := mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy()
	ctx := context.Background()
	processedImg, err := mc.processor.FitImage(ctx, srcImg, width, height, opts)
	if err != nil {
		log.Printf("[ERROR] [Monitor %d] FitImage failed with tuning %v: %v", mc.ID, opts, err)
		return
	}

	// 5. Determine derivative path and overwrite
	resKey = fmt.Sprintf("%dx%d", width, height)
	derivPath := ""
	if p, ok := img.DerivativePaths[resKey]; ok {
		derivPath = p
	} else if p, ok := img.DerivativePaths["primary"]; ok {
		derivPath = p
	}
	if derivPath == "" {
		log.Printf("[ERROR] [Monitor %d] No derivative path found for %s", mc.ID, resKey)
		return
	}

	// Remove old derivative before writing new one to force a new filesystem
	// inode. On macOS/APFS, imaging.Save → os.Create truncates in-place (same
	// inode), and the OS serves stale cached image data. Deleting first ensures
	// os.Create allocates a new inode, bypassing all file-level caching.
	os.Remove(derivPath) // Ignore error — file may not exist yet

	if err := imaging.Save(processedImg, derivPath); err != nil {
		log.Printf("[ERROR] [Monitor %d] Failed to save derivative: %v", mc.ID, err)
		return
	}

	// 6. Set wallpaper
	mc.State.CurrentImage = img
	if err := mc.os.SetWallpaper(derivPath, mc.ID); err != nil {
		log.Printf("[ERROR] [Monitor %d] Failed to set wallpaper: %v", mc.ID, err)
	}

	// 7. Refresh tray menu
	if mc.OnWallpaperChanged != nil {
		mc.OnWallpaperChanged(img, mc.ID)
	}
}
