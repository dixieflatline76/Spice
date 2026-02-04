package wallpaper

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math/rand"
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
	GetDesktopDimension() (int, int, error)
	GetMonitors() ([]Monitor, error)
	SetWallpaper(path string, monitorID int) error
	Stat(path string) (os.FileInfo, error)
}

// ImageProcessor interface defines the image processing operations.
type ImageProcessor interface {
	DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error)
	EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error)
	FitImage(ctx context.Context, img image.Image, targetWidth, targetHeight int) (image.Image, error)
	CheckCompatibility(imgWidth, imgHeight, targetWidth, targetHeight int) error
}

// MonitorMenuItems holds the tray menu items for a specific monitor.
type MonitorMenuItems struct {
	ProviderMenuItem *fyne.MenuItem
	ArtistMenuItem   *fyne.MenuItem
	FavoriteMenuItem *fyne.MenuItem
}

// Plugin is the main struct for the wallpaper downloader plugin.
type Plugin struct {
	os                  OS
	imgProcessor        ImageProcessor
	cfg                 *Config
	httpClient          *http.Client
	manager             ui.PluginManager
	downloadMutex       sync.RWMutex
	queryPages          map[string]*util.SafeCounter
	downloadedDir       string
	isDownloading       bool
	interrupt           *util.SafeFlag
	imgPulseOp          func()
	fitImageFlag        *util.SafeFlag
	shuffleImageFlag    *util.SafeFlag
	stopNightlyRefresh  chan struct{}
	ticker              *time.Ticker
	ctx                 context.Context
	cancel              context.CancelFunc
	downloadWaitGroup   *sync.WaitGroup
	prePauseFrequency   Frequency
	pauseChangeCallback func(bool)
	pauseMenuItem       *fyne.MenuItem
	providers           map[string]provider.ImageProvider
	favoriter           provider.Favoriter
	actionChan          chan func()

	// New Components
	store    *ImageStore
	fm       *FileManager
	pipeline *Pipeline

	// Configuration and State
	fetchingInProgress *util.SafeFlag

	// Session State (Local Navigation)
	lastTriggeredSeenCount  int // Anti-loop state
	lastTriggeredTotalCount int
	lastTriggerTime         time.Time // Anti-loop cooldown state

	// Monitors (Actor Model)
	Monitors    map[int]*MonitorController
	monMu       sync.RWMutex // Protects the Monitors map itself
	monitorMenu map[int]*MonitorMenuItems

	// Internal State
	enrichmentSignal chan int // Signal for lazy enrichment worker

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
		currentOS := getOS()

		baseTransport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   HTTPClientDialerTimeout,
				KeepAlive: HTTPClientKeepAlive,
			}).DialContext,
			ResponseHeaderTimeout: HTTPClientResponseHeaderTimeout,
			TLSHandshakeTimeout:   HTTPClientTLSHandshakeTimeout,
		}

		robustClient := &http.Client{
			Timeout: HTTPClientRequestTimeout,
			Transport: &UserAgentTransport{
				RoundTripper: baseTransport,
				UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
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
		store.SetOS(currentOS)

		wpInstance = &Plugin{
			os: currentOS,
			imgProcessor: NewSmartImageProcessor(
				currentOS,
				nil,
				pigoInstance,
			),
			cfg:        nil,
			httpClient: robustClient,

			downloadMutex: sync.RWMutex{},
			queryPages:    make(map[string]*util.SafeCounter),
			downloadedDir: "",
			interrupt:     util.NewSafeBoolWithValue(false),

			imgPulseOp:         nil,
			fitImageFlag:       util.NewSafeBoolWithValue(false),
			shuffleImageFlag:   util.NewSafeBoolWithValue(false),
			stopNightlyRefresh: make(chan struct{}),
			ticker:             nil,
			downloadWaitGroup:  &sync.WaitGroup{},
			providers:          make(map[string]provider.ImageProvider),
			actionChan:         make(chan func(), 5),
			store:              store,
			fetchingInProgress: util.NewSafeBool(),
			runOnUI:            fyne.Do,
			focusProviderName:  "",
		}

		wpInstance.imgPulseOp = func() { wpInstance.SetNextWallpaper(-1, true) }
	})
	return wpInstance
}

// RequestFetch safely triggers a background fetch if conditions are met.
// It implements the legacy "Anti-Loop" protection to prevent starvation loops.
func (wp *Plugin) RequestFetch() {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	// 1. Basic Debounce (Avoid spamming from UI or multiple monitors)
	if wp.isDownloading || wp.fetchingInProgress.Value() {
		return
	}

	seenCount := wp.store.SeenCount()
	totalCount := wp.store.Count()

	// 2. Anti-Loop Protection
	// CASE 1: Starvation/Dry Source
	// We triggered a download previously, but the TotalCount did NOT increase.
	// This suggests the provider returned 0 new images.
	if totalCount <= wp.lastTriggeredTotalCount && seenCount > wp.lastTriggeredSeenCount {
		// Starvation Cooldown: 60 seconds (Wait for new content or user)
		if time.Since(wp.lastTriggerTime) < 60*time.Second {
			log.Debugf("Fetch skipped: Starvation cooldown active (%v remaining). Total stuck at %d.",
				(60*time.Second - time.Since(wp.lastTriggerTime)).Round(time.Second), totalCount)
			return
		}
	} else if seenCount <= wp.lastTriggeredSeenCount {
		// CASE 2: Retrying same threshold (e.g. rapid clicks before Seen advances)
		// Cooldown Override: If it's been > 15 seconds since last trigger, allow retry.
		if time.Since(wp.lastTriggerTime) < 15*time.Second {
			log.Debugf("Fetch skipped: Debounce cooldown active (%v remaining).",
				(15*time.Second - time.Since(wp.lastTriggerTime)).Round(time.Second))
			return
		}
	}

	// 3. Trigger
	wp.lastTriggeredSeenCount = seenCount
	wp.lastTriggeredTotalCount = totalCount
	wp.lastTriggerTime = time.Now()

	// log.Printf("Triggering background fetch (Seen: %d, Total: %d)", seenCount, totalCount)
	go wp.FetchNewImages()
}

// GetInstance returns the singleton instance of the wallpaper plugin.
func GetInstance() *Plugin {
	return getPlugin()
}

// LoadPlugin initializes the wallpaper plugin and registers it with the manager.
func LoadPlugin(manager ui.PluginManager) {
	manager.Register(GetInstance())
}

// Init initializes the wallpaper plugin with the given PluginManager.
func (wp *Plugin) Init(manager ui.PluginManager) {
	wp.manager = manager
	wp.cfg = GetConfig(manager.GetPreferences())

	// Update processor config now that we have it
	if sip, ok := wp.imgProcessor.(*SmartImageProcessor); ok {
		sip.config = wp.cfg
	}

	wp.cfg.SetQueryRemovedCallback(wp.onQueryRemoved)
	wp.cfg.SetQueryDisabledCallback(wp.onQueryDisabled)

	wp.providers = make(map[string]provider.ImageProvider)
	for _, factory := range GetRegisteredProviders() {
		p := factory(wp.cfg, wp.httpClient)
		wp.providers[p.Name()] = p
		log.Debugf("Registered provider: %s", p.Name())

		if f, ok := p.(provider.Favoriter); ok {
			wp.favoriter = f
			log.Debugf("Detected Favoriter provider: %s", p.Name())
		}
	}

	downloadsPath := filepath.Join(config.GetWorkingDir(), strings.ToLower(pluginName)+"_downloads")
	wp.fm = NewFileManager(downloadsPath)

	cachePath := filepath.Join(downloadsPath, "image_cache_map.json")
	wp.store.SetFileManager(wp.fm, cachePath)
	wp.store.SetAsyncSave(true)
	wp.store.SetDebounceDuration(1 * time.Second)

	wp.Monitors = make(map[int]*MonitorController)

	wp.pipeline = NewPipeline(wp.cfg, wp.store, wp.ProcessImageJob)
	wp.enrichmentSignal = make(chan int, 1)

	log.Debugf("Wallpaper Plugin Initialized.")

	go wp.actionWorker()
	go wp.startEnrichmentWorker()
}

func (wp *Plugin) actionWorker() {
	for action := range wp.actionChan {
		action()
	}
}

func (wp *Plugin) Name() string {
	return "Wallpaper"
}

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

// Activate starts the wallpaper rotation.
func (wp *Plugin) Activate() {
	if wp.cancel != nil {
		wp.cancel()
	}
	// Create a new context for this activation cycle
	wp.ctx, wp.cancel = context.WithCancel(context.Background())

	if err := wp.fm.EnsureDirs(); err != nil {
		log.Fatalf("Error ensuring directories: %v", err)
	}

	if err := wp.store.LoadCache(); err != nil {
		log.Printf("Failed to load cache: %v", err)
	}

	// Initialize Monitors (Actors)
	monitors, err := wp.os.GetMonitors()
	if err != nil {
		log.Printf("Failed to identify monitors: %v. Using default.", err)
		monitors = []Monitor{{ID: 0, Name: "Default", Rect: image.Rect(0, 0, 1920, 1080)}} // Fallback
	}

	wp.monMu.Lock()
	// Stop existing monitors if re-activating
	for _, mc := range wp.Monitors {
		mc.Stop()
	}
	// Rebuild map
	wp.Monitors = make(map[int]*MonitorController)
	for _, m := range monitors {
		// Create actor for each monitor
		mc := NewMonitorController(m.ID, m, wp.store, wp.fm, wp.os, wp.cfg, wp.imgProcessor)
		mc.OnWallpaperChanged = func(img provider.Image, monitorID int) {
			go wp.updateTrayMenuUI(img, monitorID)
		}
		mc.OnFavoriteRequest = func(img provider.Image) {
			wp.ToggleFavorite(img)
		}
		mc.OnFetchRequest = func() {
			wp.RequestFetch()
		}
		mc.Start()
		wp.Monitors[m.ID] = mc
		log.Printf("Monitor Actor %d started: %s %v", m.ID, m.Name, m.Rect)
	}
	wp.monMu.Unlock()

	wp.syncStoreWithConfig()
	wp.store.LoadAvoidSet(wp.cfg.GetAvoidSet())

	workers := runtime.NumCPU()
	if wp.cfg != nil && wp.cfg.MaxConcurrentProcessors > 0 {
		workers = wp.cfg.MaxConcurrentProcessors
	}
	// Start Pipeline
	wp.pipeline.Start(workers)

	if wp.cfg.GetNightlyRefresh() {
		go wp.StartNightlyRefresh()
	}

	wp.SetShuffleImage(wp.cfg.GetImgShuffle())
	wp.SetSmartFit(wp.cfg.GetSmartFit())

	if wp.cfg.GetChgImgOnStart() {
		wp.RefreshImagesAndPulse()
	} else {
		// Reset all pages to 1 on clean activation if not pulsing
		wp.downloadMutex.Lock()
		for _, q := range wp.cfg.Queries {
			wp.queryPages[q.ID] = util.NewSafeIntWithValue(1)
		}
		wp.downloadMutex.Unlock()
		wp.FetchNewImages()
	}
	wp.ChangeWallpaperFrequency(wp.cfg.GetWallpaperChangeFrequency())

	// Start monitor watcher
	go wp.startMonitorWatcher()

	// Refresh tray menu to reflect discovered monitors
	log.Debugf("Activate: Requesting Tray Menu Rebuild to include discovered monitors...")
	wp.manager.RebuildTrayMenu()
}

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
	if wp.cancel != nil {
		wp.cancel()
		wp.cancel = nil
	}
	wp.interrupt.Set(true)
	wp.downloadMutex.Unlock()

	wp.stopAllWorkers()

	// Stop Pipeline
	if wp.pipeline != nil {
		wp.pipeline.Stop()
	}

	// Stop Monitors
	wp.monMu.Lock() // Write lock needed for map modification if we were to delete, but here just stopping
	for _, mc := range wp.Monitors {
		mc.Stop()
	}
	wp.monMu.Unlock()

	log.Print("Wallpaper plugin deactivated.")
}

// -------------------------------------------------------------------------
//  Command & Control Shims (Legacy Interface -> Monitor Actors)
// -------------------------------------------------------------------------

// SetNextWallpaper advances the wallpaper.
func (wp *Plugin) SetNextWallpaper(monitorID int, forceImmediate bool) {
	log.Printf("[DEBUG] SetNextWallpaper called for monitor %d (Force immediate: %v)", monitorID, forceImmediate)

	if monitorID != -1 {
		wp.dispatch(monitorID, CmdNext)
		return
	}

	// Global Ticker Trigger (-1)
	// Iterate and stagger
	wp.monMu.RLock()
	var ids []int
	for id := range wp.Monitors {
		ids = append(ids, id)
	}
	wp.monMu.RUnlock()

	stagger := wp.cfg.GetStaggerMonitorChanges()
	freq := wp.cfg.GetWallpaperChangeFrequency()
	duration := freq.Duration()

	for _, id := range ids {
		if id == 0 {
			// Primary always immediate
			wp.dispatch(id, CmdNext)
			continue
		}

		if !forceImmediate && stagger && duration > 0 {
			// Random delay 10-30% of interval
			pct := 0.1 + (rand.Float64() * 0.2)
			delay := time.Duration(float64(duration) * pct)

			log.Printf("[Stagger] Scheduling AUTOMATIC monitor %d update in %v", id, delay)

			// Capture ID for closure
			mID := id
			time.AfterFunc(delay, func() {
				log.Printf("[Stagger] Executing staggered update for monitor %d", mID)
				wp.dispatch(mID, CmdNext)
			})
		} else {
			log.Printf("[Stagger] Executing IMMEDIATE update for monitor %d (Force: %v, StaggerCfg: %v)", id, forceImmediate, stagger)
			wp.dispatch(id, CmdNext)
		}
	}
}

// SetPreviousWallpaper goes back.
func (wp *Plugin) SetPreviousWallpaper(monitorID int, forceImmediate bool) {
	log.Printf("[DEBUG] SetPreviousWallpaper called for monitor %d (Force immediate: %v)", monitorID, forceImmediate)
	wp.dispatch(monitorID, CmdPrev)
}

// DeleteCurrentImage deletes the current image on the specified monitor.
func (wp *Plugin) DeleteCurrentImage(monitorID int) {
	log.Printf("[DEBUG] DeleteCurrentImage called for monitor %d", monitorID)
	wp.dispatch(monitorID, CmdDelete)
}

// TogglePause toggles the pause state.
func (wp *Plugin) TogglePause() {
	current := wp.cfg.GetWallpaperChangeFrequency()
	if current == FrequencyNever {
		if wp.prePauseFrequency != FrequencyNever {
			wp.ChangeWallpaperFrequency(wp.prePauseFrequency)
		} else {
			wp.ChangeWallpaperFrequency(FrequencyHourly)
		}
	} else {
		wp.prePauseFrequency = current
		wp.ChangeWallpaperFrequency(FrequencyNever)
	}
	// Notify UI via callback if set
	if wp.pauseChangeCallback != nil {
		wp.pauseChangeCallback(wp.IsPaused())
	}
}

func (wp *Plugin) IsPaused() bool {
	return wp.cfg.GetWallpaperChangeFrequency() == FrequencyNever
}

func (wp *Plugin) ChangeWallpaperFrequency(newFreq Frequency) {
	wp.cfg.SetWallpaperChangeFrequency(newFreq)

	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if wp.ticker != nil {
		wp.ticker.Stop()
	}

	if newFreq != FrequencyNever {
		// Notify monitors? Generally they just react to Next commands.
		// The Plugin drives the frequency via Ticker -> SetNextWallpaper.
		duration := newFreq.Duration()
		if duration > 0 {
			wp.ticker = time.NewTicker(duration)
			// Start ticker loop
			go func() {
				// We need to capture the current ticker to stop correctly
				currentTicker := wp.ticker
				for range currentTicker.C {
					if wp.ticker != currentTicker {
						return
					}
					wp.SetNextWallpaper(-1, false)
				}
			}()
		}
	}

	wp.manager.NotifyUser("Wallpaper Change", newFreq.String())
}

func (wp *Plugin) GetOS() OS {
	return wp.os
}

// TriggerFavorite adds/removes the current image of a specific monitor from favorites.
func (wp *Plugin) TriggerFavorite(monitorID int) {
	log.Printf("[DEBUG] TriggerFavorite called for monitor %d", monitorID)
	wp.dispatch(monitorID, CmdFavorite)
}

func (wp *Plugin) TogglePauseAction() {
	wp.TogglePause()
}

func (wp *Plugin) TriggerOpenSettings() {
	if wp.manager != nil {
		wp.manager.OpenPreferences("Wallpaper")
	}
}

func (wp *Plugin) ViewCurrentImageOnWeb(monitorID int) {
	wp.monMu.RLock()
	defer wp.monMu.RUnlock()
	if mc, ok := wp.Monitors[monitorID]; ok {
		if mc.State.CurrentImage.ViewURL != "" {
			if u, err := url.Parse(mc.State.CurrentImage.ViewURL); err == nil {
				_ = fyne.CurrentApp().OpenURL(u)
			}
		}
	}
}

func (wp *Plugin) ToggleFavorite(img provider.Image) {
	if img.ID == "" {
		return
	}

	if wp.favoriter == nil {
		return
	}

	// Toggle logic
	if img.IsFavorited {
		// Remove
		if err := wp.favoriter.RemoveFavorite(img); err != nil {
			log.Printf("Failed to remove favorite: %v", err)
			return
		}
		img.IsFavorited = false
		wp.store.Update(img)
		wp.manager.NotifyUser("Favorites", "Removed from favorites.")
	} else {
		// Add
		if err := wp.favoriter.AddFavorite(img); err != nil {
			log.Printf("Failed to add favorite: %v", err)
			return
		}
		img.IsFavorited = true
		wp.store.Update(img)
		wp.manager.NotifyUser("Favorites", "Added to favorites.")
	}

	// Update UI for all monitors that might have this image
	wp.monMu.RLock()
	defer wp.monMu.RUnlock()
	for id, mc := range wp.Monitors {
		if mc.State.CurrentImage.ID == img.ID {
			// Update UI
			wp.updateTrayMenuUI(img, id)
		}
	}
}

func (wp *Plugin) SetShuffleImage(enable bool) {
	wp.cfg.SetImgShuffle(enable)
}

func (wp *Plugin) SetSmartFit(enabled bool) {
	wp.fitImageFlag.Set(enabled)
}

func (wp *Plugin) SetStaggerMonitorChanges(enable bool) {
	wp.cfg.SetStaggerMonitorChanges(enable)
}

func (wp *Plugin) StopNightlyRefresh() {
	if wp.stopNightlyRefresh != nil {
		close(wp.stopNightlyRefresh)
		wp.stopNightlyRefresh = nil
	}
}

func (wp *Plugin) ClearCache() {
	log.Println("Plugin: Clearing entire wallpaper cache...")
	wp.store.Wipe()
	log.Println("Plugin: Cache cleared. Triggering refresh...")
	go wp.RefreshImagesAndPulse()
}

// -------------------------------------------------------------------------
//  Helpers
// -------------------------------------------------------------------------

// dispatch routes commands to appropriate monitor actors.
func (wp *Plugin) dispatch(monitorID int, cmd Command) {
	wp.monMu.RLock()
	defer wp.monMu.RUnlock()

	log.Printf("[DEBUG] Dispatching command %v to monitor %d", cmd, monitorID)

	if monitorID == -1 {
		// ALL Monitors
		for _, mc := range wp.Monitors {
			select {
			case mc.Commands <- cmd:
				log.Printf("[DEBUG] Command %v sent to monitor %d buffer", cmd, mc.ID)
			default:
				log.Printf("[WARN] [Monitor %d] Command buffer full, dropping command", mc.ID)
			}
		}
	} else {
		if mc, ok := wp.Monitors[monitorID]; ok {
			select {
			case mc.Commands <- cmd:
				log.Printf("[DEBUG] Command %v sent to monitor %d buffer", cmd, monitorID)
			default:
				log.Printf("[WARN] [Monitor %d] Command buffer full, dropping command", monitorID)
			}
		} else {
			log.Printf("[WARN] Monitor %d not found for dispatch", monitorID)
		}
	}
}

// SyncMonitors reconciles the current OS display setup with our actor list.
// If force is false, it only runs a full sync if the number of monitors has changed.
func (wp *Plugin) SyncMonitors(force bool) {
	current, err := wp.os.GetMonitors()
	if err != nil {
		log.Printf("[ERROR] DynamicSync: Failed to get monitors: %v", err)
		return
	}

	wp.monMu.Lock()
	existingCount := len(wp.Monitors)
	wp.monMu.Unlock()

	if !force && len(current) == existingCount {
		// Quick check passed: monitor count is identical
		// Future: could check resolution of each DevicePath here too
		return
	}

	log.Printf("[Sync] Display setup changed or forced sync. Existing: %d, Current: %d", existingCount, len(current))

	wp.monMu.Lock()
	changed := wp.syncMonitorsLocked(current, force)
	wp.monMu.Unlock()

	if changed {
		log.Print("[Sync] Display setup synchronized. Triggering Tray Rebuild.")
		wp.manager.RebuildTrayMenu()
	}
}

// syncMonitorsLocked performs the monitor reconciliation while holding the lock.
func (wp *Plugin) syncMonitorsLocked(current []Monitor, force bool) bool {
	existingCount := len(wp.Monitors)
	if !force && len(current) == existingCount {
		return false
	}

	log.Printf("[Sync] Display setup changed or forced sync. Existing: %d, Current: %d", existingCount, len(current))

	// Map to track what we currently have
	foundPaths := make(map[string]bool)
	changed := false

	// 1. Add/Update Monitors
	for _, m := range current {
		foundPaths[m.DevicePath] = true

		if mc, exists := wp.Monitors[m.ID]; exists {
			// Check for resolution change on the same ID
			oldRect := mc.Monitor.Rect
			if oldRect != m.Rect {
				log.Printf("[Sync] Resolution change for Monitor %d: %v -> %v", m.ID, oldRect, m.Rect)
				mc.Monitor = m
				changed = true
				// Trigger refresh for new resolution
				go wp.SetNextWallpaper(m.ID, true)
			}
		} else {
			// New Monitor
			log.Printf("[Sync] New Monitor detected: %d (%s) at %v", m.ID, m.Name, m.Rect)
			mc := NewMonitorController(m.ID, m, wp.store, wp.fm, wp.os, wp.cfg, wp.imgProcessor)
			mc.OnWallpaperChanged = func(img provider.Image, monitorID int) {
				go wp.updateTrayMenuUI(img, monitorID)
			}
			mc.OnFavoriteRequest = func(img provider.Image) {
				wp.ToggleFavorite(img)
			}
			mc.OnFetchRequest = func() {
				wp.RequestFetch()
			}
			mc.Start()
			wp.Monitors[m.ID] = mc
			changed = true
			// Trigger pulse for NEW display
			go wp.SetNextWallpaper(m.ID, true)
		}
	}

	// 2. Remove Stale Monitors
	for id, mc := range wp.Monitors {
		stillExists := false
		for _, m := range current {
			if m.ID == id {
				stillExists = true
				break
			}
		}

		if !stillExists {
			log.Printf("[Sync] Monitor %d removed. Stopping actor.", id)
			mc.Stop()
			delete(wp.Monitors, id)
			changed = true
		}
	}

	// 3. Force UI Refresh for ALL active monitors
	// This ensures that even if monitors persisted, their menu state (attribution) is restored/verified.
	for id, mc := range wp.Monitors {
		// Use the current image state
		go wp.updateTrayMenuUI(mc.State.CurrentImage, id)
	}

	return changed
}

func (wp *Plugin) startMonitorWatcher() {
	ticker := time.NewTicker(90 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wp.ctx.Done():
			log.Debug("[Sync] Stopping monitor watcher (Context Cancelled)")
			return
		case <-ticker.C:
			wp.SyncMonitors(false)
		}
	}
}

func (wp *Plugin) updateTrayMenuUI(img provider.Image, monitorID int) {
	wp.runOnUI(func() {
		wp.monMu.RLock()
		mItems, ok := wp.monitorMenu[monitorID]
		wp.monMu.RUnlock()
		if !ok {
			return
		}

		attribution := img.Attribution
		if len(attribution) > 20 {
			attribution = attribution[:17] + "..."
		}
		mItems.ProviderMenuItem.Label = "Source: " + wp.GetProviderTitle(img.Provider)
		mItems.ProviderMenuItem.Action = func() {
			wp.focusProviderName = img.Provider
			wp.manager.OpenPreferences("Wallpaper")
		}

		// Restore Icons using the provider abstraction
		if p, exists := wp.providers[img.Provider]; exists {
			mItems.ProviderMenuItem.Icon = p.GetProviderIcon()
		} else {
			mItems.ProviderMenuItem.Icon = nil
		}

		mItems.ArtistMenuItem.Label = "By: " + attribution
		if attribution == "" {
			mItems.ArtistMenuItem.Label = "By: Unknown"
		}

		// Update Favorite State
		if mItems.FavoriteMenuItem != nil {
			if img.IsFavorited {
				mItems.FavoriteMenuItem.Label = "Remove from Favorites"
				mItems.FavoriteMenuItem.Icon, _ = wp.manager.GetAssetManager().GetIcon("unfavorite.png")
			} else {
				mItems.FavoriteMenuItem.Label = "Add to Favorites"
				mItems.FavoriteMenuItem.Icon, _ = wp.manager.GetAssetManager().GetIcon("favorite.png")
			}
		}

		wp.manager.RefreshTrayMenu()
	})
}

func (wp *Plugin) onQueryRemoved(queryID string) {
	log.Printf("Plugin: Query %s removed. Clearing...", queryID)
	wp.store.RemoveByQueryID(queryID)
	wp.downloadMutex.Lock()
	delete(wp.queryPages, queryID)
	wp.downloadMutex.Unlock()
}

func (wp *Plugin) onQueryDisabled(queryID string) {
	log.Printf("Plugin: Query %s disabled. Clearing from cache/rotation...", queryID)
	wp.store.RemoveByQueryID(queryID)
	wp.downloadMutex.Lock()
	delete(wp.queryPages, queryID)
	wp.downloadMutex.Unlock()
}

// syncStoreWithConfig reconciles the image store with the current configuration.
// It handles query activation, cache sizing, and derivative invalidation for processing modes.
func (wp *Plugin) syncStoreWithConfig() {
	mode := wp.cfg.GetSmartFitMode()
	targetFlags := map[string]bool{
		"SmartFit":       wp.cfg.GetSmartFit(),
		"FitFlexibility": mode == SmartFitAggressive,
		"FitQuality":     mode == SmartFitNormal,
		"FaceCrop":       wp.cfg.GetFaceCropEnabled(),
		"FaceBoost":      wp.cfg.GetFaceBoostEnabled(),
	}

	wp.store.Sync(int(wp.cfg.GetCacheSize().Size()), targetFlags, wp.cfg.GetActiveQueryIDs())
}

func (wp *Plugin) GetProviderTitle(providerID string) string {
	if p, ok := wp.providers[providerID]; ok {
		return p.Title()
	}
	return providerID
}

func (wp *Plugin) startEnrichmentWorker() {
	for range wp.enrichmentSignal {
		// Stub: Process enrichment queues
	}
}

// StartNightlyRefresh shim if not in scheduler.go (It should be in scheduler.go according to lints)
// We call wp.StartNightlyRefresh() which is defined in scheduler.go
// So we don't define it here.

// OpenAddCollectionUI parses the given URL and opens the settings panel to add it as a query.
func (wp *Plugin) OpenAddCollectionUI(testURL string) error {
	for name, p := range wp.providers {
		if !p.SupportsUserQueries() {
			continue
		}
		result, err := p.ParseURL(testURL)
		if err == nil && result != "" {
			wp.pendingAddUrl = testURL
			wp.focusProviderName = name
			if wp.manager != nil {
				wp.manager.OpenPreferences("Wallpaper")
			}
			return nil
		}
	}
	return fmt.Errorf("no provider found that can handle this URL")
}
