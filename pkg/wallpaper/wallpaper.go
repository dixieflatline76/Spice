package wallpaper

import (
	"context"
	"fmt"
	"image"
	"math/rand"

	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/util"
	"github.com/dixieflatline76/Spice/util/log"
	pigo "github.com/esimov/pigo/core"
)

// OS interface defines the operating system specific operations.
type OS interface {
	getDesktopDimension() (int, int, error)
	setWallpaper(path string) error
}

// ImageProcessor interface defines the image processing operations.
type ImageProcessor interface {
	DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error)
	EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error)
	FitImage(ctx context.Context, img image.Image) (image.Image, error)
	CheckCompatibility(width, height int) error
}

// Plugin is the main struct for the wallpaper downloader plugin.
type Plugin struct {
	os                  OS
	imgProcessor        ImageProcessor
	cfg                 *Config
	httpClient          *http.Client
	manager             ui.PluginManager
	downloadMutex       sync.RWMutex
	currentDownloadPage *util.SafeCounter
	downloadedDir       string
	isDownloading       bool
	interrupt           *util.SafeFlag
	currentImage        provider.Image
	imgPulseOp          func()
	fitImageFlag        *util.SafeFlag
	shuffleImageFlag    *util.SafeFlag
	stopNightlyRefresh  chan struct{}
	ticker              *time.Ticker
	cancel              context.CancelFunc
	downloadWaitGroup   *sync.WaitGroup
	prePauseFrequency   Frequency
	pauseChangeCallback func(bool)
	pauseMenuItem       *fyne.MenuItem
	providerMenuItem    *fyne.MenuItem
	artistMenuItem      *fyne.MenuItem
	favoriteMenuItem    *fyne.MenuItem
	providers           map[string]provider.ImageProvider
	favoriter           provider.Favoriter
	actionChan          chan func()

	// New Components
	store    *ImageStore
	fm       *FileManager
	pipeline *Pipeline

	// Concurrency Control
	fetchingInProgress *util.SafeFlag

	// Session State (Local Navigation)
	currentIndex            int
	history                 []int
	shuffleOrder            []int
	randomPos               int
	lastTriggeredSeenCount  int // Anti-loop state
	lastTriggeredTotalCount int
	lastTriggerTime         time.Time // Anti-loop cooldown state
	enrichmentSignal        chan int  // Signal for lazy enrichment worker

	// Testable UI executor
	runOnUI func(func())

	pendingAddUrl     string
	focusProviderName string             // State to focus specific provider settings
	settingsTabs      *container.AppTabs // Reference to the settings tabs for dynamic switching
}

var (
	wpInstance *Plugin
	wpOnce     sync.Once
)

// getPlugin returns the singleton instance of the wallpaper plugin.
func getPlugin() *Plugin {
	wpOnce.Do(func() {
		// Initialize the wallpaper service for right OS
		currentOS := getOS()

		robustClient := &http.Client{
			Timeout: HTTPClientRequestTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   HTTPClientDialerTimeout,
					KeepAlive: HTTPClientKeepAlive,
				}).DialContext,
				ResponseHeaderTimeout: HTTPClientResponseHeaderTimeout,
				TLSHandshakeTimeout:   HTTPClientTLSHandshakeTimeout,
			},
		}

		// Initialize pigo
		var pigoInstance *pigo.Pigo
		am := asset.NewManager()
		modelData, err := am.GetModel("facefinder")
		if err != nil {
			log.Printf("Warning: Failed to load face detection model: %v. Face Boost will be disabled.", err)
		} else {
			p := pigo.NewPigo()
			pigoInstance, err = p.Unpack(modelData)
			if err != nil {
				log.Printf("Warning: Failed to unpack face detection model: %v. Face Boost will be disabled.", err)
				pigoInstance = nil
			}
		}

		store := NewImageStore()

		// Initialize the wallpaper service
		wpInstance = &Plugin{
			os: currentOS, // Initialize with right OS
			imgProcessor: &smartImageProcessor{
				os:              currentOS,
				aspectThreshold: 0.9,
				resampler:       imaging.Lanczos,
				pigo:            pigoInstance,
				config:          nil, // Will be set in Init
			},
			cfg:        nil,
			httpClient: robustClient,

			downloadMutex:       sync.RWMutex{},
			currentDownloadPage: util.NewSafeIntWithValue(1), // Start with the first page,
			downloadedDir:       "",
			interrupt:           util.NewSafeBoolWithValue(false),

			// imgPulseOp will be set to the *internal* method, which should be called FROM the worker.
			imgPulseOp:         nil,
			fitImageFlag:       util.NewSafeBoolWithValue(false),
			shuffleImageFlag:   util.NewSafeBoolWithValue(false),
			stopNightlyRefresh: make(chan struct{}),
			ticker:             nil,
			downloadWaitGroup:  &sync.WaitGroup{},
			providers:          make(map[string]provider.ImageProvider),
			actionChan:         make(chan func(), 5),
			store:              store,
			// pipeline:           nil, // Initialized in Init
			fetchingInProgress: util.NewSafeBool(),
			runOnUI:            fyne.Do,
			focusProviderName:  "",
		}

		// Pipeline depends on wpInstance methods, so we init it here but config is nil.
		// We should perhaps init it in Init() when config is available?
		// Or pass wpInstance to it?
		// NewPipeline takes cfg.
		// We'll init pipeline in Init().

		wpInstance.imgPulseOp = wpInstance.setNextWallpaper
	})
	return wpInstance
}

// Init initializes the wallpaper plugin with the given PluginManager.
func (wp *Plugin) Init(manager ui.PluginManager) {
	wp.manager = manager
	wp.cfg = GetConfig(manager.GetPreferences())

	// Register callbacks
	wp.cfg.SetQueryRemovedCallback(wp.onQueryRemoved)

	// Initialize providers via Registry
	wp.providers = make(map[string]provider.ImageProvider)
	for _, factory := range GetRegisteredProviders() {
		p := factory(wp.cfg, wp.httpClient)
		wp.providers[p.Name()] = p
		log.Debugf("Registered provider: %s", p.Name())

		// Detect Favoriter
		if f, ok := p.(provider.Favoriter); ok {
			wp.favoriter = f
			log.Debugf("Detected Favoriter provider: %s", p.Name())
		}
	}

	// Initialize FileManager
	downloadsPath := filepath.Join(config.GetWorkingDir(), strings.ToLower(pluginName)+"_downloads")
	wp.fm = NewFileManager(downloadsPath)

	// Configure Store with Persistence
	cachePath := filepath.Join(downloadsPath, "image_cache_map.json")
	wp.store.SetFileManager(wp.fm, cachePath)

	// Enable Async Persistence to debounce disk writes during batch downloads
	wp.store.SetAsyncSave(true)
	wp.store.SetDebounceDuration(1 * time.Second)

	// Inject config into smartImageProcessor
	if sip, ok := wp.imgProcessor.(*smartImageProcessor); ok {
		sip.config = wp.cfg
	}

	// Initialize Pipeline
	wp.pipeline = NewPipeline(wp.cfg, wp.store, wp.ProcessImageJob)
	wp.enrichmentSignal = make(chan int, 1) // Buffered 1 for non-blocking signal

	log.Debugf("Wallpaper Plugin Initialized. Config: FaceBoostEnabled=%v, SmartFit=%v", wp.cfg.GetFaceBoostEnabled(), wp.cfg.GetSmartFit())

	// Start the action worker and enrichment worker
	// Start the action worker and enrichment worker
	go wp.actionWorker()
	go wp.startEnrichmentWorker()
}

// actionWorker processes wallpaper change actions sequentially.
func (wp *Plugin) actionWorker() {
	for action := range wp.actionChan {
		action()
	}
}

// Name returns the name of the plugin.
func (wp *Plugin) Name() string {
	return "Wallpaper"
}

// stopAllWorkers stops all workers and cancels any ongoing downloads. It blocks until all workers have stopped.
func (wp *Plugin) stopAllWorkers() {

	log.Print("Stopping all workers...")
	wp.interrupt.Set(true)
	wp.downloadMutex.Lock()
	if wp.cancel != nil {
		wp.cancel()
		wp.cancel = nil
	}
	wg := wp.downloadWaitGroup
	wp.downloadMutex.Unlock()

	if wg != nil {
		log.Print("Waiting for running downloads to stop...")
		wg.Wait()
		log.Print("All running downloads stopped.")
	}
}

// DeleteCurrentImage deletes the current wallpaper image from the filesystem and updates the history.
func (wp *Plugin) DeleteCurrentImage() {
	if wp.currentImage.FilePath == "" {
		return
	}

	idToDelete := wp.currentImage.ID

	// Persist to AvoidSet (Config)
	wp.cfg.AddToAvoidSet(idToDelete)

	// Send delete command to State Manager
	if wp.pipeline != nil {
		wp.pipeline.SendCommand(StateCmd{Type: CmdRemove, Payload: idToDelete})
	}

	// Delete file from disk
	err := os.Remove(wp.currentImage.FilePath)
	if err != nil {
		log.Printf("Failed to delete file %s: %v", wp.currentImage.FilePath, err)
	} else {
		log.Printf("Deleted file: %s", wp.currentImage.FilePath)
	}

	// Update local state
	// Remove this index from history if it exists
	// Ideally we find the index of this ID if still valid?
	// Simplified: Just move to next wallpaper

	// We might need to rebuild shuffle order if we want to be strict,
	// but moving to next image is safe.
	wp.SetNextWallpaper()
}

// Activate starts the wallpaper rotation.
func (wp *Plugin) Activate() {
	if err := wp.fm.EnsureDirs(); err != nil {
		log.Fatalf("Error ensuring directories: %v", err)
	}

	// Load Cache Logic
	if err := wp.store.LoadCache(); err != nil {
		log.Printf("Failed to load cache: %v", err)
	}

	targetFlags := map[string]bool{
		"SmartFit": wp.cfg.GetSmartFit(),
		"FaceCrop": wp.cfg.GetFaceCropEnabled(),
	}
	wp.store.Sync(int(wp.cfg.GetCacheSize().Size()), targetFlags, wp.cfg.GetActiveQueryIDs())

	// Sync Blocklist from Config to Store
	wp.store.LoadAvoidSet(wp.cfg.GetAvoidSet())

	// Start the pipeline with default worker count (NumCPU)
	// We should probably get this from config, but for now hardcode or use NumCPU
	workers := runtime.NumCPU()
	if wp.cfg != nil && wp.cfg.MaxConcurrentProcessors > 0 {
		workers = wp.cfg.MaxConcurrentProcessors
	}
	wp.pipeline.Start(workers)

	if wp.cfg.GetNightlyRefresh() {
		wp.downloadMutex.Lock()
		if wp.stopNightlyRefresh == nil {
			wp.stopNightlyRefresh = make(chan struct{})
			log.Print("Recreated nightly refresh stop channel.")
		}
		go wp.startNightlyRefresher()
		wp.downloadMutex.Unlock()
	}

	wp.SetShuffleImage(wp.cfg.GetImgShuffle())
	wp.SetSmartFit(wp.cfg.GetSmartFit())

	if wp.cfg.GetChgImgOnStart() {
		wp.RefreshImagesAndPulse()
	} else {
		wp.currentDownloadPage.Set(1)
		wp.downloadAllImages(nil)
	}
	wp.ChangeWallpaperFrequency(wp.cfg.GetWallpaperChangeFrequency())
}

// changeFrequency changes the wallpaper change frequency.
func (wp *Plugin) changeFrequency(newFrequency Frequency) {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if wp.ticker != nil {
		wp.ticker.Stop()
	}

	if newFrequency == FrequencyNever {
		wp.manager.NotifyUser("Wallpaper Change", "Disabled")
		return
	}

	wp.ticker = time.NewTicker(newFrequency.Duration())

	// Capture the ticker locally to avoid racing on wp.ticker field if it changes
	ticker := wp.ticker

	go func() {
		for range ticker.C {
			wp.actionChan <- func() {
				wp.downloadMutex.Lock()
				op := wp.imgPulseOp
				wp.downloadMutex.Unlock()
				if op != nil {
					// op is already one of the Set*Wallpaper methods which now push to channel?
					// Wait, if imgPulseOp is SetNextWallpaper, calling it here pushes to channel.
					// That's fine. But wait, SetNextWallpaper logs "User triggered".
					// We might want separate or just reuse.
					// Actually, SetNextWallpaper (public) calls imgPulseOp (internal op).
					// If imgPulseOp is setNextWallpaper (internal), it does the work.
					// So we should wrap it.
					// But `imgPulseOp` is currently `setNextWallpaper` or `setRandomWallpaper`.
					// Those are the internal methods doing the work.
					// So we should just call it directly here?
					// No, we are INSIDE the actionWorker context (if we push this closure).
					// YES.
					op()
				}
			}
		}
	}()
	wp.manager.NotifyUser("Wallpaper Change", newFrequency.String())
}

// Deactivate stops the wallpaper rotation, any active downloads, and cleans up.
func (wp *Plugin) Deactivate() {
	log.Print("Deactivating wallpaper plugin...")

	wp.downloadMutex.Lock()
	if wp.stopNightlyRefresh != nil {
		close(wp.stopNightlyRefresh)
		wp.stopNightlyRefresh = nil
		log.Print("Nightly refresh stop signal sent and channel cleared.")
	}
	if wp.ticker != nil {
		wp.ticker.Stop()
		log.Print("Wallpaper change ticker stopped.")
	}
	wp.interrupt.Set(true)
	wp.downloadMutex.Unlock()

	wp.stopAllWorkers()

	log.Print("Wallpaper plugin deactivated.")
}

// getWallhavenURL returns the wallhaven URL for the given API URL.
func (wp *Plugin) getWallhavenURL(apiURL string) *url.URL {
	urlStr := strings.Replace(apiURL, "https://wallhaven.cc/api/v1/search?", "https://wallhaven.cc/search?", 1)
	url, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}

	q := url.Query()
	if !q.Has("resolutions") && !q.Has("atleast") {
		if width, height, err := wp.os.getDesktopDimension(); err == nil {
			q.Set("atleast", fmt.Sprintf("%dx%d", width, height))
		}
	}
	url.RawQuery = q.Encode()
	return url
}

// updateTrayMenuUI updates the tray menu items with the given image details.
// It runs on the UI thread.
func (wp *Plugin) updateTrayMenuUI(img provider.Image) {
	wp.runOnUI(func() {
		if wp.providerMenuItem != nil && wp.artistMenuItem != nil {
			attribution := img.Attribution
			if len(attribution) > 20 {
				attribution = attribution[:17] + "..."
			}
			providerName := wp.GetProviderTitle(img.Provider)
			wp.providerMenuItem.Label = "Source: " + providerName
			wp.providerMenuItem.Action = func() {
				// Focus Provider Settings instead of opening website
				if _, ok := wp.providers[img.Provider]; ok {
					go func() {
						if err := wp.FocusProviderSettings(img.Provider); err != nil {
							log.Printf("Failed to focus provider settings: %v", err)
						}
					}()
				} else {
					// Fallback for unknown providers (e.g. from old version or manual add)
					homeURL := "https://github.com/dixieflatline76/Spice"
					if u, err := url.Parse(homeURL); err == nil {
						if err := wp.manager.OpenURL(u); err != nil {
							log.Printf("Failed to open URL %s: %v", homeURL, err)
						}
					}
				}
			}

			// Update Icon
			var icon fyne.Resource
			if provider, ok := wp.providers[img.Provider]; ok {
				icon = provider.GetProviderIcon()
			}
			// Fallback to default if no specific icon
			if icon == nil {
				icon, _ = wp.manager.GetAssetManager().GetIcon("provider_default.png")
			}
			wp.providerMenuItem.Icon = icon

			wp.artistMenuItem.Label = "By: " + attribution
			if attribution == "" {
				wp.artistMenuItem.Label = "By: Unknown"
			}
			wp.updateFavoriteMenuItem(false)
			wp.manager.RefreshTrayMenu()
		}
	})
}

// applyWallpaper sets the wallpaper to the given image and triggers necessary logic.
func (wp *Plugin) applyWallpaper(img provider.Image) {
	start := time.Now()
	// Trigger download event (async)
	if img.DownloadLocation != "" {
		go wp.triggerDownload(img.DownloadLocation)
	}

	// Optimistic UI Update: Update tray menu and internal state BEFORE setting wallpaper.
	// This fixes the URL desync issue where "View on Web" would link to the old image while blocks.
	prevImage := wp.currentImage
	wp.currentImage = img
	go wp.updateTrayMenuUI(img)

	// Mark as seen to update history/pagination logic (Async to avoid blocking UI)
	go wp.store.MarkSeen(img.FilePath)

	// Verify file exists before setting
	if _, err := os.Stat(img.FilePath); os.IsNotExist(err) {
		log.Printf("Error: Wallpaper file missing: %s", img.FilePath)
		// Don't rollback immediately, just abort. User might have deleted it manually.
		return
	}

	// log.Debugf("Applying Wallpaper: ID=%s, Provider=%s, Path=%s", img.ID, img.Provider, img.FilePath)

	osStart := time.Now()
	if err := wp.os.setWallpaper(img.FilePath); err != nil {
		log.Printf("failed to set wallpaper: %v", err)
		// Rollback UI and state to previous image
		wp.currentImage = prevImage
		if wp.currentImage.FilePath != "" {
			wp.updateTrayMenuUI(wp.currentImage)
		}
		return
	}
	log.Debugf("[Latency] Wallpaper transition for %s: Total=%v (Worker=%v, OS=%v)",
		img.ID, time.Since(start), osStart.Sub(start), time.Since(osStart))

	// Threshold logic: If we have seen > 80% of local images (approximation), fetch next page.
	seenCount := wp.store.SeenCount()
	totalCount := wp.store.Count()

	if totalCount == 0 {
		return
	}

	threshold := int(math.Round(PrcntSeenTillDownload * float64(totalCount)))

	// debug: dump precise pagination state
	// log.Debugf("Pagination Check: Seen=%d, Total=%d, Threshold=%d, LastTrigger=%d, InProgress=%v",
	// 	seenCount, totalCount, threshold, wp.lastTriggeredSeenCount, wp.fetchingInProgress.Value())

	// Pagination Logic:
	// User Request: "retrieve the next page once the user has seen 70% of the page's images"
	// We use totalCount as proxy for "page's images" (or total loaded).
	// Current logic uses PrcntSeenTillDownload (likely 0.7 or 0.8).
	shouldFetch := false

	if totalCount > 0 {
		// If we have enough images to care about percentage:
		if seenCount >= threshold {
			shouldFetch = true
		}

		// Safety: If totalCount is very small (e.g. < MinSeenImagesForDownload), calculation might be weird.
		// But 70% of 4 is 3. So if seen 3/4, we fetch. This seems correct.
		// The old MinSeen check was blocking this.
	}

	if shouldFetch {
		// Anti-Loop Check: ensure we are making progress

		// CASE 1: Starvation/Dry Source
		// We triggered a download previously, but the TotalCount did NOT increase.
		// This suggests the provider returned 0 new images (duplicates or end of list).
		// If we simply check 'seen > lastTrigger', we will infinite loop because Seen keeps growing.
		// We must enforce a cooldown if TotalCount is stuck.
		if totalCount <= wp.lastTriggeredTotalCount && seenCount > wp.lastTriggeredSeenCount {
			// Starvation Cooldown: 60 seconds (Wait for new content or user to back off)
			// This is critical for providers like MetMuseum with strict filtering or limited paging.
			if time.Since(wp.lastTriggerTime) > 60*time.Second {
				log.Debugf("Pagination: Starvation cooldown expired (%v). Retrying download.", time.Since(wp.lastTriggerTime))
				shouldFetch = true
			} else {
				log.Debugf("Pagination: Starvation prevented. Total (%d) stuck. Seen (%d) advancing. Waiting for cooldown (%v).",
					totalCount, seenCount, (60*time.Second - time.Since(wp.lastTriggerTime)).Round(time.Second))
				shouldFetch = false
			}
		} else if seenCount <= wp.lastTriggeredSeenCount {
			// CASE 2: Retrying same threshold (e.g. rapid clicks before Seen advances)
			// Cooldown Override: If it's been > 15 seconds since last trigger, allow retry.
			if time.Since(wp.lastTriggerTime) > 15*time.Second {
				log.Debugf("Pagination: Loop prevention cooldown expired (%v). Retrying download despite stalled count.", time.Since(wp.lastTriggerTime))
				shouldFetch = true
			} else {
				log.Debugf("Pagination: Loop prevented. Seen=%d, LastTrigger=%d. Waiting for new views (Cooldown: %v remaining).",
					seenCount, wp.lastTriggeredSeenCount, (15*time.Second - time.Since(wp.lastTriggerTime)).Round(time.Second))
				shouldFetch = false
			}
		}
	}

	if shouldFetch {
		wp.downloadMutex.Lock()
		// Double check inside lock (legacy), but mainly check atomic flag
		if !wp.isDownloading {
			// Atomic Check: Prevent overlapping fetches from rapid UI triggers
			if wp.fetchingInProgress.CompareAndSwap(false, true) {
				wp.isDownloading = true
				wp.lastTriggeredSeenCount = seenCount
				wp.lastTriggeredTotalCount = totalCount // Trace total at time of trigger
				wp.lastTriggerTime = time.Now()
				wp.downloadMutex.Unlock()

				// log.Debugf("Seen %d/%d images (Trigger: %v). Fetching next page... (CAS Success)", seenCount, totalCount, shouldFetch)
				wp.currentDownloadPage.Increment()

				// Trigged async download
				go func() {
					defer func() {
						log.Debugf("Pagination: Async download finished. Resetting flag.")
						wp.fetchingInProgress.Set(false)
					}()
					wp.downloadAllImages(nil)
				}()
			} else {
				wp.downloadMutex.Unlock()
				log.Debugf("Pagination: Fetch skipped - already in progress (CAS Failed). Seen=%d", seenCount)
			}
		} else {
			wp.downloadMutex.Unlock()
			log.Debugf("Pagination: Fetch skipped - isDownloading=true mutex check.")
		}
	}
}

// setWallpaperAt is deprecated/removed. Logic moved to applyWallpaper.
// setRandomWallpaper is removed. Logic moved to Store.

func (wp *Plugin) setNextWallpaper() {
	// DEBUG: Dump state to find logic disconnect
	// wp.store.DumpState()

	count := wp.store.Count()
	if count == 0 {
		log.Println("No wallpapers available. Triggering refresh...")
		go wp.RefreshImagesAndPulse()
		return
	}

	var newIndex int
	if wp.shuffleImageFlag.Value() {
		// Rebuild shuffle order if needed
		if len(wp.shuffleOrder) != count || wp.randomPos >= len(wp.shuffleOrder) {
			log.Debugf("Rebuilding Shuffle Order. Count: %d, RandomPos: %d", count, wp.randomPos)
			wp.rebuildShuffleOrder(count)
		}
		newIndex = wp.shuffleOrder[wp.randomPos]
		// log.Debugf("Shuffle Selection: Index %d from position %d/%d", newIndex, wp.randomPos, count)
		wp.randomPos++
	} else {
		newIndex = (wp.currentIndex + 1) % count
		// log.Debugf("Sequential Selection: Index %d (Current: %d, Count: %d)", newIndex, wp.currentIndex, count)
	}

	img, ok := wp.store.Get(newIndex)
	if !ok {
		log.Printf("Failed to get image at index %d", newIndex)
		return
	}

	wp.currentIndex = newIndex
	wp.history = append(wp.history, newIndex)

	wp.applyWallpaper(img)
}

// GetOS returns the underlying OS interface.
func (wp *Plugin) GetOS() OS {
	return wp.os
}

// OpenAddCollectionUI opens the preferences window and prompts to add the collection.
func (wp *Plugin) OpenAddCollectionUI(urlStr string) error {
	// 1. Identify Provider (Validation)
	var foundProvider provider.ImageProvider
	for _, p := range wp.providers {
		// Skip providers that don't support custom queries (e.g. curated museums)
		if !p.SupportsUserQueries() {
			continue
		}
		if _, err := p.ParseURL(urlStr); err == nil {
			foundProvider = p
			break
		}
	}
	if foundProvider == nil {
		return fmt.Errorf("URL not supported by any active provider")
	}

	// 2. Set State
	wp.pendingAddUrl = urlStr

	// 3. Open Preferences (Outer Layer)
	wp.manager.OpenPreferences("Wallpaper")

	// 4. Switch Inner Tab (Inner Layer)
	// If the settings panel is already built, switch logic in CreatePrefsPanel won't run.
	// We must manually switch it here.
	if wp.settingsTabs != nil {
		targetTabIndex := 1 // Default to Online
		switch foundProvider.Type() {
		case provider.TypeLocal:
			targetTabIndex = 2 // Local
		case provider.TypeAI:
			targetTabIndex = 3 // AI
		}

		// Ensure we are on UI thread? Fyne methods are usually safe if called from event cycle,
		// but this might be called from API server goroutine.
		// OpenPreferences handles UI thread via `sa.NewWindow`.
		// We should wrap this in runOnUI just in case.
		wp.runOnUI(func() {
			if targetTabIndex < len(wp.settingsTabs.Items) {
				wp.settingsTabs.SelectIndex(targetTabIndex)
			}
		})
	}

	return nil
}

// FocusProviderSettings opens the preferences and focuses the settings for the given provider.
func (wp *Plugin) FocusProviderSettings(providerName string) error {
	// 1. Verify Provider
	p, ok := wp.providers[providerName]
	if !ok {
		return fmt.Errorf("provider %s not found", providerName)
	}

	// 2. Set State
	wp.focusProviderName = providerName

	// 3. Open Preferences (Outer Layer)
	wp.manager.OpenPreferences("Wallpaper")

	// 4. Switch Inner Tab (Inner Layer)
	if wp.settingsTabs != nil {
		targetTabIndex := 1 // Default to Online
		switch p.Type() {
		case provider.TypeLocal:
			targetTabIndex = 2 // Local
		case provider.TypeAI:
			targetTabIndex = 3 // AI
		}

		wp.runOnUI(func() {
			if targetTabIndex < len(wp.settingsTabs.Items) {
				wp.settingsTabs.SelectIndex(targetTabIndex)
			}
		})
	}
	return nil
}

// setPrevWallpaper sets the previous wallpaper.
func (wp *Plugin) setPrevWallpaper() {
	if len(wp.history) <= 1 {
		log.Println("No previous wallpaper available.")
		wp.manager.NotifyUser("Wallpaper", "No previous wallpaper available.")
		return
	}

	// Pop current
	wp.history = wp.history[:len(wp.history)-1]
	// Get previous
	newIndex := wp.history[len(wp.history)-1]

	// Adjust random pos if needed
	if wp.shuffleImageFlag.Value() && wp.randomPos > 0 {
		wp.randomPos--
	}

	img, ok := wp.store.Get(newIndex)
	if !ok {
		log.Printf("Failed to get image at index %d", newIndex)
		return
	}

	wp.currentIndex = newIndex
	wp.applyWallpaper(img)
}

func (wp *Plugin) rebuildShuffleOrder(count int) {
	if count == 0 {
		return
	}
	// Simple deterministic shuffle based on current seed/time
	// In a real app we might want to persist this or be more clever.
	// For now, simple rand.Perm is fine as it stays local.
	// Note: rand.Perm is pseudo-random.
	wp.shuffleOrder = rand.Perm(count)
	wp.randomPos = 0
}

// SetPreviousWallpaper sets the previous wallpaper in the list.
// This is a PUBLIC method acting as a Trigger.
func (wp *Plugin) SetPreviousWallpaper() {
	// enqueueTime := time.Now()
	// log.Debugf("[Timing] SetPreviousWallpaper: Enqueueing action (Queue: %d)", len(wp.actionChan))
	wp.actionChan <- func() {
		// startExec := time.Now()
		// log.Debugf("[Timing] SetPreviousWallpaper: Starting. Waited: %v", startExec.Sub(enqueueTime))

		wp.setPrevWallpaper()

		// log.Debugf("[Timing] SetPreviousWallpaper: Finished. Duration: %v", time.Since(startExec))
	}
}

// SetNextWallpaper sets the next wallpaper, will respect shuffle toggle
// This is a PUBLIC method acting as a Trigger.
func (wp *Plugin) SetNextWallpaper() {
	// enqueueTime := time.Now()
	// log.Debugf("[Timing] SetNextWallpaper: Enqueueing action (Queue: %d)", len(wp.actionChan))
	wp.actionChan <- func() {
		// startExec := time.Now()
		// log.Debugf("[Timing] SetNextWallpaper: Starting. Waited: %v", startExec.Sub(enqueueTime))

		wp.imgPulseOp()

		// Signal the enrichment worker to pivot to the new index
		if wp.enrichmentSignal != nil {
			select {
			case wp.enrichmentSignal <- wp.currentIndex:
			default:
				// If channel is full (worker busy), drain and replace
				select {
				case <-wp.enrichmentSignal:
				default:
				}
				wp.enrichmentSignal <- wp.currentIndex
			}
		}

		// log.Debugf("[Timing] SetNextWallpaper: Finished. Duration: %v", time.Since(startExec))
	}
}

// GetInstance returns the singleton instance of the wallpaper plugin.
func GetInstance() *Plugin {
	return getPlugin()
}

// TogglePause toggles the wallpaper change frequency between paused (Never) and the previous frequency.
func (wp *Plugin) TogglePause() {
	// wp.downloadMutex.Lock() // Removed as part of refactor
	currentFreq := wp.cfg.GetWallpaperChangeFrequency()
	// wp.downloadMutex.Unlock()

	if currentFreq == FrequencyNever {
		// Resume
		if wp.prePauseFrequency != FrequencyNever {
			wp.cfg.SetWallpaperChangeFrequency(wp.prePauseFrequency)
			wp.ChangeWallpaperFrequency(wp.prePauseFrequency)
		} else {
			wp.cfg.SetWallpaperChangeFrequency(FrequencyHourly)
			wp.ChangeWallpaperFrequency(FrequencyHourly)
		}
		if wp.pauseChangeCallback != nil {
			wp.pauseChangeCallback(false)
		}
	} else {
		// Pause
		wp.prePauseFrequency = currentFreq
		wp.cfg.SetWallpaperChangeFrequency(FrequencyNever)
		wp.ChangeWallpaperFrequency(FrequencyNever)
		if wp.pauseChangeCallback != nil {
			wp.pauseChangeCallback(true)
		}
	}
}

// TogglePauseAction triggers the UI action for pausing/resuming if available, otherwise toggles logic directly.
func (wp *Plugin) TogglePauseAction() {
	if wp.pauseMenuItem != nil {
		wp.pauseMenuItem.Action()
	} else {
		wp.TogglePause()
	}
}

// SetPauseChangeCallback sets the callback function to be called when pause state changes.
func (wp *Plugin) SetPauseChangeCallback(callback func(bool)) {
	// wp.downloadMutex.Lock()
	// defer wp.downloadMutex.Unlock()
	wp.pauseChangeCallback = callback
}

// IsPaused returns true if the wallpaper change frequency is set to Never.
func (wp *Plugin) IsPaused() bool {
	return wp.cfg.GetWallpaperChangeFrequency() == FrequencyNever
}

// SetRandomWallpaper is removed. Logic handled by Store.

// GetCurrentImage returns the current wallpaper image information.
func (wp *Plugin) GetCurrentImage() provider.Image {
	return wp.currentImage
}

// TriggerFavorite adds the current image to favorites if supported, respecting Strict Add logic.
// This is used by the Global Hotkey.
func (wp *Plugin) TriggerFavorite() {
	if wp.favoriter == nil {
		// Log or notify? Silent failure is probably best if feature unavailable.
		return
	}
	img := wp.currentImage
	if img.ID == "" {
		return
	}

	// Strict Add Check
	if wp.favoriter.IsFavorited(img) {
		wp.manager.NotifyUser("Favorite", "Already in favorites")
		return
	}

	if err := wp.favoriter.AddFavorite(img); err != nil {
		log.Printf("Failed to add favorite via hotkey: %v", err)
		wp.manager.NotifyUser("Favorite", "Failed to save")
		return
	}

	wp.manager.NotifyUser("Favorite", "Added to favorites")

	// Update UI state (e.g. tray checkmark)
	go func() {
		// Small delay to allow file system propagation if needed, though updateTrayMenuUI is robust.
		wp.runOnUI(func() {
			if wp.providerMenuItem != nil {
				wp.updateFavoriteMenuItem(false)
			}
		})
	}()
}

// TriggerOpenSettings is a helper to open the main preferences window.
// This is used by the Global Hotkey.
func (wp *Plugin) TriggerOpenSettings() {
	if wp.manager != nil {
		wp.manager.OpenPreferences("")
	}
}

// ChangeWallpaperFrequency changes the wallpaper frequency.
func (wp *Plugin) ChangeWallpaperFrequency(newFrequency Frequency) {
	wp.changeFrequency(newFrequency)
}

// ViewCurrentImageOnWeb opens the current image in the default browser.
func (wp *Plugin) ViewCurrentImageOnWeb() {
	if wp.currentImage.ViewURL == "" {
		log.Println("no current image to view.")
		return
	}
	url, err := url.Parse(wp.currentImage.ViewURL)
	if err != nil {
		log.Printf("failed to parse URL: %v", err)
		return
	}
	if err := wp.manager.OpenURL(url); err != nil {
		log.Printf("failed to open URL: %v", err)
	}
}

// downloadAllImages downloads images from all active URLs (Producer).
func (wp *Plugin) downloadAllImages(doneChan chan struct{}) {
	wp.interrupt.Set(false)

	if wp.currentDownloadPage.Value() <= 1 {
		wp.store.Clear()
	}

	ctx, cancel := context.WithTimeout(context.Background(), HTTPClientRequestTimeout)
	// We need to keep context alive for the duration of DISCOVERY.
	defer cancel()

	wp.downloadMutex.Lock()
	wp.isDownloading = true
	queries := wp.cfg.GetQueries()
	wp.downloadMutex.Unlock()

	defer func() {
		wp.downloadMutex.Lock()
		wp.isDownloading = false
		wp.downloadMutex.Unlock()
		if doneChan != nil {
			close(doneChan)
		}
	}()

	var message string
	log.Debugf("Producing jobs for %d queries...", len(queries))

	var wg sync.WaitGroup
	for _, query := range queries {
		if query.Active {
			wg.Add(1)
			go func(q ImageQuery) {
				defer wg.Done()
				wp.produceJobsForURL(ctx, q, wp.currentDownloadPage.Value()) // Async producer
			}(query)
			message += fmt.Sprintf("[%s]\n", query.Description)
		}
	}
	wg.Wait()
	wp.manager.NotifyUser("Downloading: ", message)

	// Cold Start Enrichment: Trigger the worker for the initial set
	// We use non-blocking send in case worker is already busy/started
	if wp.enrichmentSignal != nil {
		select {
		case wp.enrichmentSignal <- 0: // Start at index 0
		default:
		}
	}
}

// startEnrichmentWorker runs a persistent loop to handle lazy enrichment.
// It pivots immediately when a new index is signaled.
func (wp *Plugin) startEnrichmentWorker() {
	for newIndex := range wp.enrichmentSignal {
		// We have a new starting point.
		// Define window size
		windowSize := 5
		startIdx := newIndex
		count := wp.store.Count()
		if count == 0 {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		processed := 0

		// Look ahead loop (including current image at i=0)
		for i := 0; i <= windowSize; i++ {
			// CHECK FOR PIVOT: Before doing work, check if user moved again
			select {
			case latestIndex := <-wp.enrichmentSignal:
				log.Debugf("Lazy Enrichment: Pivoting from %d to %d", startIdx, latestIndex)
				// Updates loop variables to restart immediately
				startIdx = latestIndex
				i = 0 // Reset loop (will increment to 1)
				continue
			default:
				// No new signal, proceed with current window
			}

			idx := (startIdx + i) % count
			img, ok := wp.store.Get(idx)
			if !ok || img.Attribution != "" {
				continue
			}

			// Needs enrichment
			p, ok := wp.providers[img.Provider]
			if !ok {
				continue
			}

			// Perform Enrichment
			// log.Debugf("Lazy Enrichment: Fetching metadata for %s...", img.ID)
			enriched, err := p.EnrichImage(ctx, img)
			if err == nil && enriched.Attribution != "" {
				wp.store.Update(enriched)
				processed++

				// If this is the current image, update the UI labels immediately
				if idx == wp.currentIndex {
					wp.currentImage = enriched
					wp.updateTrayMenuUI(enriched)
				}
			}
			// Small pacing
			time.Sleep(100 * time.Millisecond)
		}
		cancel()
		if processed > 0 {
			log.Debugf("Lazy Enrichment: Updated %d images in window %d.", processed, startIdx)
		}
	}
}

// produceJobsForURL (Producer helper)
func (wp *Plugin) produceJobsForURL(ctx context.Context, query ImageQuery, page int) {
	// Find provider
	var downloadProvider provider.ImageProvider
	if query.Provider != "" {
		if p, ok := wp.providers[query.Provider]; ok {
			downloadProvider = p
		}
	}

	if downloadProvider == nil {
		log.Printf("No active provider found for query: %s (Provider: %s)", query.ID, query.Provider)
		return
	}

	apiURL, _ := downloadProvider.ParseURL(query.URL)

	// Resolution check
	if rap, ok := downloadProvider.(provider.ResolutionAwareProvider); ok {
		width, height, err := wp.os.getDesktopDimension()
		if err == nil {
			apiURL = rap.WithResolution(apiURL, width, height)
		}
	}

	images, err := downloadProvider.FetchImages(ctx, apiURL, page)
	if err != nil {
		log.Printf("Failed to fetch from %s: %v", downloadProvider.Name(), err)
		return
	}

	for _, img := range images {
		// Strict Block Check: Don't process if user blocked it.
		if wp.cfg.InAvoidSet(img.ID) {
			log.Debugf("Skipping blocked image %s", img.ID)
			continue
		}

		img.SourceQueryID = query.ID // Set SourceQueryID for smart clearance
		// Smart Producer Check:
		// If the image is already in our store (validated by Sync), we check if it needs "claiming".
		if existing, ok := wp.store.GetByID(img.ID); ok {
			// If existing image has no SourceQueryID (Legacy or Lost Metadata), claim it for this query.
			// This ensures Strict Sync can manage it later.
			if existing.SourceQueryID == "" {
				existing.SourceQueryID = query.ID
				wp.store.Add(existing) // Update the store with the tagged image
				log.Debugf("Claimed legacy/untagged image %s for query %s", img.ID, query.ID)
			}
			continue
		}

		job := DownloadJob{
			Image:    img,
			Provider: downloadProvider,
		}
		if !wp.pipeline.Submit(job) {
			log.Printf("Pipeline full or stopped. Dropping image %s", img.ID)
			return
		}
	}
}

// RefreshImagesAndPulse refreshes the list of images and pulses the image.
func (wp *Plugin) RefreshImagesAndPulse() {
	// Run asynchronously to avoid blocking
	go func() {
		// Strict Sync: Prune inactive/deleted queries before fetching new ones.
		// This ensures that "Apply" button or Manual Refresh immediately cleans up the cache.
		targetFlags := map[string]bool{
			"SmartFit": wp.cfg.GetSmartFit(),
			"FaceCrop": wp.cfg.GetFaceCropEnabled(),
		}
		wp.store.Sync(int(wp.cfg.GetCacheSize().Size()), targetFlags, wp.cfg.GetActiveQueryIDs())

		// CRITICAL: Reset playback state via Action Worker.
		// Since the Store was just synced, the indices in shuffleOrder and history might now be invalid (out of bounds)
		// or pointing to the wrong images (shifted). We must force a rebuild.
		doneReset := make(chan struct{})
		wp.actionChan <- func() {
			log.Println("State: Resetting playback history and shuffle order due to refresh.")
			wp.shuffleOrder = nil
			wp.history = nil // Or keep history? No, indices are invalid.
			wp.randomPos = 0
			// wp.currentIndex remains, but verify it?
			// If currentIndex is out of bounds, SetNextWallpaper handles it?
			// Ideally we don't change wallpaper immediately, just the *future* order.
			// But since history is cleared, previous button breaks.
			// That is acceptable for a "Refresh/Apply" action.
			close(doneReset)
		}
		<-doneReset

		wp.currentDownloadPage.Set(1)

		downloadDone := make(chan struct{})
		// This will run discovery synchronously or asynchronously depending on implementation,
		// but since we passed a channel, our `downloadAllImages` closes it when DISCOVERY is done.
		// Wait, `produceJobsForURL` is async in my implementation above? Yes `go wp.produceJobsForURL`.
		// So `downloadAllImages` returns almost immediately after spawning producers.
		// And `defer close(doneChan)` happens on return.
		// So `doneChan` closes immediately?
		// No, `downloadAllImages` spawns producers but doesn't wait for them?
		// Correct. To wait for discovery, `downloadAllImages` should wait for producers.
		// I'll update `downloadAllImages` to wait.
		wp.downloadAllImages(downloadDone)

		<-downloadDone

		// Now we wait for *Processing*?
		// The requirement is "Pulse AS SOON AS we have enough images".
		// We can poll the store.

		timeout := time.After(MaxImageWaitRetry * ImageWaitRetryDelay)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				wp.actionChan <- func() { wp.imgPulseOp() }
				return
			case <-ticker.C:
				if wp.store.Count() >= MinLocalImageBeforePulse {
					wp.actionChan <- func() { wp.imgPulseOp() }
					return
				}
			}
		}
	}()
}

// SetSmartFit enables or disables smart cropping.
func (wp *Plugin) SetSmartFit(enabled bool) {
	wp.fitImageFlag.Set(enabled)
}

// SetShuffleImage enables or disables random wallpaper selection.
func (wp *Plugin) SetShuffleImage(enable bool) {
	wp.shuffleImageFlag.Set(enable)
	wp.cfg.SetImgShuffle(enable) // Persist setting
	if enable {
		count := wp.store.Count()
		wp.rebuildShuffleOrder(count)
		wp.imgPulseOp = wp.setNextWallpaper
	} else {
		wp.imgPulseOp = wp.setNextWallpaper
	}
}

// checkWallhavenURL takes a transformed API URL and its type, performs a network check
// checkWallhavenURL and CheckWallhavenURL are removed as they are legacy logic.
// Validation should be handled by the provider itself.

// CheckWallhavenURL checks if the given URL is a valid Wallhaven URL.

// GetWallhavenURL returns the Wallhaven URL for the given API URL.
func (wp *Plugin) GetWallhavenURL(apiURL string) *url.URL {
	return wp.getWallhavenURL(apiURL)
}

// StopNightlyRefresh signals the nightly refresh goroutine to stop.
func (wp *Plugin) StopNightlyRefresh() {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if wp.stopNightlyRefresh != nil {
		close(wp.stopNightlyRefresh) // Signal the goroutine to stop
		wp.stopNightlyRefresh = nil  // Set to nil so we don't close it twice
		log.Print("Nightly refresh stop signal sent and channel cleared.")
	}
}

// StartNightlyRefresh starts the goroutine for nightly wallpaper refresh.
func (wp *Plugin) StartNightlyRefresh() {
	// Stop any existing goroutine before starting a new one.
	wp.StopNightlyRefresh()

	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	// Create a new stop channel and start the goroutine
	wp.stopNightlyRefresh = make(chan struct{})
	log.Print("Created new nightly refresh stop channel.")
	go wp.startNightlyRefresher()
}

// LoadPlugin loads the wallpaper plugin.
func LoadPlugin(pm ui.PluginManager) {
	pm.Register(getPlugin())
}

// triggerDownload sends a request to the provider's download location endpoint to register the download.
func (wp *Plugin) triggerDownload(url string) {
	ctx, cancel := context.WithTimeout(context.Background(), NetworkConnectivityCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("Failed to create download trigger request: %v", err)
		return
	}

	// Unsplash requires the Client-ID to be sent with the download trigger request
	// But we removed UnsplashClientID from this package.
	// We can't access it here. Unsplash trigger logic should be in Unsplash provider?
	// But triggerDownload is generic?
	// The `download_location` URL from Unsplash API typically includes the client_id as query param when fetched?
	// No, Unsplash guidelines say "Hit the links.download_location endpoint".
	// And "You must also send your Access Key via the Authorization header or client_id query param".
	// Since we don't have Unsplash logic here, we rely on the provider.
	// But `Plugin` handles downloads generally.
	// If `Image` struct had a `TriggerDownload()` method callback?
	// For now, we skip detailed trigger auth or assume URL has params.
	// Or we make `EnrichImage` or similar handle it?
	// `triggerDownload` is called by `downloadImage`.
	// We will just remove the Unsplash specific header injection here to break dependency.
	// If Unsplash needs it, we should add `DownloadHeaders` map to `Image` struct?
	// Or `HeaderProvider` interface for download trigger?
	// `HeaderProvider` was for `Download` (image). This is `triggerDownload`.

	// Just logging usage for now.

	resp, err := wp.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to trigger download event: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Download trigger returned status: %d", resp.StatusCode)
	} else {
		log.Debugf("Download event triggered successfully for %s", url)
	}
}

// onQueryRemoved is a callback triggered when a query is removed from config.
func (wp *Plugin) onQueryRemoved(queryID string) {
	log.Printf("Plugin: Query %s removed. Clearing associated images from cache...", queryID)
	wp.store.RemoveByQueryID(queryID)
	// Refresh Tray Menu or perform other syncs if needed
}

// ClearCache clears the entire wallpaper cache (memory and disk).
// This is the "Panic Button" functionality.
func (wp *Plugin) ClearCache() {
	log.Println("Plugin: Clearing entire wallpaper cache...")
	wp.store.Wipe()

	// Automatically trigger a refresh to repopulate if possible
	log.Println("Plugin: Cache cleared. Triggering refresh to repopulate...")
	go wp.RefreshImagesAndPulse()
}

// GetProviderTitle returns the display title of a provider, falling back to ID.
func (wp *Plugin) GetProviderTitle(providerID string) string {
	if p, ok := wp.providers[providerID]; ok {
		return p.Title()
	}
	return providerID
}

// isFavorited checks if the current image is in the favorites collection.
func (wp *Plugin) isFavorited() bool {
	if wp.favoriter == nil {
		return false
	}
	return wp.favoriter.IsFavorited(wp.currentImage)
}

// ToggleFavorite adds or removes the current image from the favorites collection.
func (wp *Plugin) ToggleFavorite() {
	if wp.favoriter == nil || wp.currentImage.FilePath == "" {
		return
	}

	if wp.isFavorited() {
		// Unfavorite
		wp.currentImage.IsFavorited = false
		wp.store.Update(wp.currentImage)

		if err := wp.favoriter.RemoveFavorite(wp.currentImage); err != nil {
			log.Printf("Failed to remove favorite: %v", err)
			return
		}
		wp.manager.NotifyUser("Favorites", "Removed from favorites.")

		// Aggressive Cleanup: Remove from store immediately (synchronous)
		// This prevents SetNextWallpaper from picking the same image if it's the only one.
		wp.store.Remove(wp.currentImage.ID)

		// Auto-Skip: Move to next wallpaper immediately
		go wp.SetNextWallpaper()
	} else {
		// Favorite

		// Try to enrich image if attribution is missing (e.g. Wallhaven lazily loaded)
		if wp.currentImage.Attribution == "" && wp.currentImage.Provider != "" {
			if p, ok := wp.providers[wp.currentImage.Provider]; ok {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if enriched, err := p.EnrichImage(ctx, wp.currentImage); err == nil && enriched.Attribution != "" {
					wp.currentImage.Attribution = enriched.Attribution
					log.Printf("Enriched image %s before favoriting: %s", wp.currentImage.ID, wp.currentImage.Attribution)
				}
				cancel()
			}
		}

		if err := wp.favoriter.AddFavorite(wp.currentImage); err != nil {
			log.Printf("Failed to add favorite: %v", err)
			return
		}
		wp.currentImage.IsFavorited = true
		wp.currentImage.SourceQueryID = wp.favoriter.GetSourceQueryID()
		wp.store.Update(wp.currentImage)

		wp.manager.NotifyUser("Favorites", "Added to favorites.")

		// Auto-Activate: If favorites query is not active, enable it
		favQueryID := wp.favoriter.GetSourceQueryID()
		if q, exists := wp.cfg.GetQuery(favQueryID); !exists || !q.Active {
			log.Println("Auto-activating Favorites source...")
			// We still use AddFavoritesQuery for convenience as it handles the logic
			// but we could also just call cfg.EnableImageQuery if it exists.
			if _, err := wp.cfg.AddFavoritesQuery("Favorite Images", favQueryID, true); err != nil {
				log.Printf("Failed to auto-activate Favorites: %v", err)
			}
			wp.RefreshImagesAndPulse()
		}
	}

	// Update UI
	wp.updateTrayMenuUI(wp.currentImage) // This includes updateFavoriteMenuItem(false) and Refresh
}

// updateFavoriteMenuItem updates the label and icon of the favorite menu item.
func (wp *Plugin) updateFavoriteMenuItem(refresh bool) {
	wp.runOnUI(func() {
		if wp.favoriteMenuItem == nil || wp.favoriter == nil {
			return
		}

		// Visibility: Only visible if Favorites query is active
		favQueryID := wp.favoriter.GetSourceQueryID()
		q, exists := wp.cfg.GetQuery(favQueryID)
		if !exists || !q.Active {
			// If it's currently showing and should be hidden, we need to rebuild the menu
			// Actually, CreateTrayMenuItems will filter it out if we update it.
			// Let's ensure CreateTrayMenuItems handles this.
			wp.favoriteMenuItem = nil // Reset so it's not reused if hidden?
			// RebuildTrayMenu is called by whoever toggles the query
			return
		}

		if wp.isFavorited() {
			wp.favoriteMenuItem.Label = "Unfavorite Image"
			if icon, err := wp.cfg.GetAssetManager().GetIcon("unfavorite.png"); err == nil {
				wp.favoriteMenuItem.Icon = icon
			} else {
				wp.favoriteMenuItem.Icon = theme.DeleteIcon() // Fallback
			}
		} else {
			wp.favoriteMenuItem.Label = "Add to Favorites"
			if icon, err := wp.cfg.GetAssetManager().GetIcon("favorite.png"); err == nil {
				wp.favoriteMenuItem.Icon = icon
			} else {
				wp.favoriteMenuItem.Icon = theme.ContentAddIcon() // Fallback
			}
		}

		if refresh {
			wp.manager.RefreshTrayMenu()
		}
	})
}
