package wallpaper

import (
	"context"
	"fmt"
	"image"

	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
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
}

// Plugin is the main struct for the wallpaper downloader plugin.
type Plugin struct {
	os                   OS
	imgProcessor         ImageProcessor
	cfg                  *Config
	httpClient           *http.Client
	manager              ui.PluginManager
	downloadMutex        sync.RWMutex
	currentDownloadPage  *util.SafeCounter
	downloadedDir        string
	isDownloading        bool
	localImgRecs         []provider.Image
	interrupt            *util.SafeFlag
	currentImage         provider.Image
	localImgIndex        util.SafeCounter
	randomizedIndexes    []int
	randomizedIndexesPos int
	seenImages           map[string]bool
	prevLocalImgs        []int
	imgPulseOp           func()
	fitImageFlag         *util.SafeFlag
	shuffleImageFlag     *util.SafeFlag
	stopNightlyRefresh   chan struct{}
	ticker               *time.Ticker
	cancel               context.CancelFunc
	downloadWaitGroup    *sync.WaitGroup
	prePauseFrequency    Frequency
	pauseChangeCallback  func(bool)
	pauseMenuItem        *fyne.MenuItem
	providerMenuItem     *fyne.MenuItem
	artistMenuItem       *fyne.MenuItem
	providers            map[string]provider.ImageProvider
	actionChan           chan func()
}

type fileInfo struct {
	path    string
	modTime time.Time
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
			localImgRecs:        []provider.Image{},
			interrupt:           util.NewSafeBoolWithValue(false),

			localImgIndex:        *util.NewSafeIntWithValue(-1),
			randomizedIndexes:    []int{},
			randomizedIndexesPos: 0,
			seenImages:           make(map[string]bool),
			prevLocalImgs:        []int{},
			// imgPulseOp will be set to the *internal* method, which should be called FROM the worker.
			imgPulseOp:         nil,
			fitImageFlag:       util.NewSafeBoolWithValue(false),
			shuffleImageFlag:   util.NewSafeBoolWithValue(false),
			stopNightlyRefresh: make(chan struct{}), // Initialize the channel for nightly refresh
			providers:          make(map[string]provider.ImageProvider),
			actionChan:         make(chan func(), 5), // Buffered channel for actions
		}
		wpInstance.imgPulseOp = wpInstance.setNextWallpaper
	})
	return wpInstance
}

// Init initializes the wallpaper plugin with the given PluginManager.
func (wp *Plugin) Init(manager ui.PluginManager) {
	wp.manager = manager
	wp.cfg = GetConfig(manager.GetPreferences())

	// Initialize providers via Registry
	wp.providers = make(map[string]provider.ImageProvider)
	for _, factory := range GetRegisteredProviders() {
		provider := factory(wp.cfg, wp.httpClient)
		wp.providers[provider.Name()] = provider
		log.Debugf("Registered provider: %s", provider.Name())
	}

	// Inject config into smartImageProcessor
	if sip, ok := wp.imgProcessor.(*smartImageProcessor); ok {
		sip.config = wp.cfg
	}

	log.Debugf("Wallpaper Plugin Initialized. Config: FaceBoostEnabled=%v, SmartFit=%v", wp.cfg.GetFaceBoostEnabled(), wp.cfg.GetSmartFit())

	// Start the action worker
	go wp.actionWorker()
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

// resetPluginState clears all state related to image downloads and resets the plugin. It also cleans up any downloaded images from the cache directory.
func (wp *Plugin) resetPluginState() {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	wp.localImgRecs = []provider.Image{} // Clear the download history.
	clear(wp.seenImages)                 // Clear the seen history.
	wp.prevLocalImgs = []int{}           // Clear the previous history.
	wp.currentDownloadPage.Set(1)        // Reset to the first page.
	wp.localImgIndex.Set(-1)             // Reset the current image index.

	err := wp.cleanupImageCache()
	if err != nil {
		log.Printf("error clearing downloaded images directory: %v", err)
	}
}

// setWallpaperAt sets the wallpaper at the specified index.
func (wp *Plugin) setWallpaperAt(imageIndex int) {
	var shouldDownloadNextPage bool

	wp.downloadMutex.RLock()
	// log.Debugf("[Timing] setWallpaperAt: RLock acquired after %v", time.Since(t0))
	if len(wp.localImgRecs) == 0 {
		log.Println("no downloaded images found.")
		wp.downloadMutex.RUnlock()
		return
	}

	seenCount := len(wp.seenImages)
	localRecsCount := float64(len(wp.localImgRecs))
	threshold := int(math.Round(PrcntSeenTillDownload * localRecsCount))

	if seenCount > MinSeenImagesForDownload && seenCount >= threshold {
		shouldDownloadNextPage = true
	}

	safeIndex := (imageIndex + len(wp.localImgRecs)) % len(wp.localImgRecs)
	isi := wp.localImgRecs[safeIndex]
	imagePath := isi.FilePath
	wp.downloadMutex.RUnlock()

	if shouldDownloadNextPage {
		wp.downloadMutex.Lock()
		if !wp.isDownloading {
			wp.currentDownloadPage.Increment()
			wp.isDownloading = true
			go wp.downloadAllImages(nil)
		}
		wp.downloadMutex.Unlock()
	}

	// Update internal state and UI *before* setting the wallpaper to ensure responsiveness
	// and avoid race conditions where the menu update happens long after the user action.
	// Update internal state
	wp.downloadMutex.Lock()
	// log.Debugf("[Timing] setWallpaperAt: Lock acquired in %v", time.Since(tLockStart))

	wp.currentImage = isi
	wp.localImgIndex.Set(safeIndex)
	wp.seenImages[imagePath] = true
	wp.downloadMutex.Unlock()

	// Update UI *after* releasing the lock to prevent deadlock.
	// (RefreshTrayMenu might block waiting for UI thread, which might be waiting for the lock)
	if wp.providerMenuItem != nil && wp.artistMenuItem != nil {
		attribution := isi.Attribution
		if attribution == "" {
			attribution = "Unknown"
		}
		if len(attribution) > 20 {
			attribution = attribution[:17] + "..."
		}
		wp.providerMenuItem.Label = "Source: " + isi.Provider
		wp.providerMenuItem.Action = func() {
			var homeURL string
			if p, ok := wp.providers[isi.Provider]; ok {
				homeURL = p.HomeURL()
			} else {
				homeURL = "https://github.com/dixieflatline76/Spice"
			}
			if homeURL != "" {
				if u, err := url.Parse(homeURL); err == nil {
					if err := fyne.CurrentApp().OpenURL(u); err != nil {
						log.Printf("Failed to open URL %s: %v", homeURL, err)
					}
				}
			}
		}

		// Update Icon
		var icon fyne.Resource
		if provider, ok := wp.providers[isi.Provider]; ok {
			icon = provider.GetProviderIcon()
		}
		// Fallback to default if no specific icon
		if icon == nil {
			icon, _ = wp.manager.GetAssetManager().GetIcon("provider_default.png")
		}
		wp.providerMenuItem.Icon = icon

		wp.artistMenuItem.Label = "By: " + attribution
		wp.manager.RefreshTrayMenu()
	}

	if err := wp.os.setWallpaper(imagePath); err != nil {
		log.Printf("failed to set wallpaper: %v", err)
		// Note: We don't revert the UI state here because it would be jarring.
		// The next successful wallpaper change will correct it.
		return
		return
	}
	// log.Debugf("[Timing] setWallpaperAt: OS SetWallpaper took %v", time.Since(tOSStart))

	// Trigger download event for Unsplash (or other providers requiring it)
	if isi.DownloadLocation != "" {
		go wp.triggerDownload(isi.DownloadLocation)
	}

	// log.Debugf("[Timing] setWallpaperAt: Total internal duration %v", time.Since(t0))
}

// DeleteCurrentImage deletes the current wallpaper image from the filesystem and updates the history.
func (wp *Plugin) DeleteCurrentImage() {
	if wp.localImgIndex.Value() == -1 {
		log.Println("no current image to delete.")
		return
	}

	imagePath := wp.currentImage.FilePath

	wp.downloadMutex.Lock()
	currentPos := wp.localImgIndex.Value()
	wp.localImgRecs = append(wp.localImgRecs[:currentPos], wp.localImgRecs[currentPos+1:]...)
	wp.prevLocalImgs = wp.prevLocalImgs[:len(wp.prevLocalImgs)-1]
	delete(wp.seenImages, imagePath)
	wp.cfg.AddToAvoidSet(wp.currentImage.ID)
	wp.localImgIndex.Decrement()
	wp.downloadMutex.Unlock()

	wp.manager.NotifyUser("Wallpaper Blocked", "Image deleted and added to blocklist.")

	wp.manager.NotifyUser("Wallpaper Blocked", "Image deleted and added to blocklist.")

	wp.SetNextWallpaper()

	if err := os.Remove(imagePath); err != nil {
		log.Printf("failed to delete blocked image: %v", err)
	}
}

// cleanupImageCache clears the downloaded images directory.
func (wp *Plugin) cleanupImageCache() error {
	var files []fileInfo
	dirs := []string{
		wp.downloadedDir,
		filepath.Join(wp.downloadedDir, FittedImgDir),
		filepath.Join(wp.downloadedDir, FittedFaceBoostImgDir),
		filepath.Join(wp.downloadedDir, FittedFaceCropImgDir),
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// If the directory doesn't exist, just skip it
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("error reading directory %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && isImageFile(entry.Name()) {
				info, err := entry.Info()
				if err != nil {
					return err
				}
				files = append(files, fileInfo{filepath.Join(dir, entry.Name()), info.ModTime()})
			}
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	excess := len(files) - wp.cfg.GetCacheSize().Size()
	if excess > 0 {
		for i := 0; i < excess; i++ {
			err := os.Remove(files[i].path)
			if err != nil {
				return fmt.Errorf("error deleting file %s: %w", files[i].path, err)
			}
		}
	}
	return nil
}

// setupImageDirs sets up the downloaded images directories.
func (wp *Plugin) setupImageDirs() {
	wp.downloadedDir = filepath.Join(config.GetWorkingDir(), strings.ToLower(pluginName)+"_downloads")
	fittedDir := filepath.Join(wp.downloadedDir, FittedImgDir)
	fittedFaceBoostDir := filepath.Join(wp.downloadedDir, FittedFaceBoostImgDir)
	fittedFaceCropDir := filepath.Join(wp.downloadedDir, FittedFaceCropImgDir)

	err := os.MkdirAll(wp.downloadedDir, 0755)
	if err != nil {
		log.Fatalf("error creating downloaded images directory: %v", err)
	}
	err = os.MkdirAll(fittedDir, 0755)
	if err != nil {
		log.Fatalf("error creating fitted images directory: %v", err)
	}
	err = os.MkdirAll(fittedFaceBoostDir, 0755)
	if err != nil {
		log.Fatalf("error creating fitted face boost images directory: %v", err)
	}
	err = os.MkdirAll(fittedFaceCropDir, 0755)
	if err != nil {
		log.Fatalf("error creating fitted face crop images directory: %v", err)
	}
}

// Activate starts the wallpaper rotation.
func (wp *Plugin) Activate() {
	wp.setupImageDirs()

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

// getCurrentImage returns the current image.
func (wp *Plugin) getCurrentImage() provider.Image {
	wp.downloadMutex.RLock()
	defer wp.downloadMutex.RUnlock()
	return wp.currentImage
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

// setNextWallpaper sets the next wallpaper in the list.
// This is the INTERNAL method that does the work. It should be called from the worker.
func (wp *Plugin) setNextWallpaper() {
	wp.downloadMutex.Lock()
	// log.Debugf("[Timing] setNextWallpaper: Lock acquired in %v", time.Since(t0))
	newIndex := wp.localImgIndex.Increment()
	wp.prevLocalImgs = append(wp.prevLocalImgs, newIndex)
	wp.downloadMutex.Unlock()

	wp.setWallpaperAt(newIndex)
}

// setRandomWallpaper sets a random wallpaper from the list.
// This is the INTERNAL method.
func (wp *Plugin) setRandomWallpaper() {
	wp.downloadMutex.Lock()
	// log.Debugf("[Timing] setRandomWallpaper: Lock acquired in %v", time.Since(t0))
	if len(wp.localImgRecs) == 0 {
		log.Println("no downloaded images found.")
		wp.downloadMutex.Unlock()
		return
	}
	if len(wp.randomizedIndexes) != len(wp.localImgRecs) || wp.randomizedIndexesPos >= len(wp.randomizedIndexes) {
		wp.randomizedIndexes = rand.Perm(len(wp.localImgRecs))
		wp.randomizedIndexesPos = 0
	}
	randomIndex := wp.randomizedIndexes[wp.randomizedIndexesPos]
	wp.randomizedIndexesPos++
	wp.prevLocalImgs = append(wp.prevLocalImgs, randomIndex)
	wp.downloadMutex.Unlock()
	wp.setWallpaperAt(randomIndex)
}

// SetPreviousWallpaper sets the previous wallpaper in the list.
// This is a PUBLIC method acting as a Trigger.
func (wp *Plugin) SetPreviousWallpaper() {
	// enqueueTime := time.Now()
	// log.Debugf("[Timing] SetPreviousWallpaper: Enqueueing action (Queue: %d)", len(wp.actionChan))
	wp.actionChan <- func() {
		// startExec := time.Now()
		// log.Debugf("[Timing] SetPreviousWallpaper: Starting. Waited: %v", startExec.Sub(enqueueTime))

		wp.downloadMutex.Lock()
		if len(wp.prevLocalImgs) <= 1 {
			wp.downloadMutex.Unlock()
			wp.manager.NotifyUser("No Previous Wallpaper", "You are at the beginning.")
			return
		}
		wp.prevLocalImgs = wp.prevLocalImgs[:len(wp.prevLocalImgs)-1]
		tempIndex := wp.prevLocalImgs[len(wp.prevLocalImgs)-1]
		if wp.shuffleImageFlag.Value() {
			wp.randomizedIndexesPos--
		}
		wp.downloadMutex.Unlock()
		wp.setWallpaperAt(tempIndex)

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

		// log.Debugf("[Timing] SetNextWallpaper: Finished. Duration: %v", time.Since(startExec))
	}
}

// GetInstance returns the singleton instance of the wallpaper plugin.
func GetInstance() *Plugin {
	return getPlugin()
}

// TogglePause toggles the wallpaper change frequency between paused (Never) and the previous frequency.
func (wp *Plugin) TogglePause() {
	wp.downloadMutex.Lock()
	currentFreq := wp.cfg.GetWallpaperChangeFrequency()
	wp.downloadMutex.Unlock()

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
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()
	wp.pauseChangeCallback = callback
}

// IsPaused returns true if the wallpaper change frequency is set to Never.
func (wp *Plugin) IsPaused() bool {
	return wp.cfg.GetWallpaperChangeFrequency() == FrequencyNever
}

// SetRandomWallpaper sets a random wallpaper.
func (wp *Plugin) SetRandomWallpaper() {
	wp.actionChan <- func() {
		wp.setRandomWallpaper()
	}
}

// GetCurrentImage returns the current wallpaper image information.
func (wp *Plugin) GetCurrentImage() provider.Image {
	return wp.getCurrentImage()
}

// ChangeWallpaperFrequency changes the wallpaper frequency.
func (wp *Plugin) ChangeWallpaperFrequency(newFrequency Frequency) {
	wp.changeFrequency(newFrequency)
}

// ViewCurrentImageOnWeb opens the current image in the default browser.
func (wp *Plugin) ViewCurrentImageOnWeb() {
	wp.downloadMutex.RLock()
	defer wp.downloadMutex.RUnlock()
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

// RefreshImagesAndPulse refreshes the list of images and pulses the image.
func (wp *Plugin) RefreshImagesAndPulse() {
	go func() {
		wp.currentDownloadPage.Set(1)

		// Use a channel to track download completion asynchronously
		downloadDone := make(chan struct{})
		wp.downloadAllImages(downloadDone)

		// Wait for either enough images or completion/timeout
		// This loop allows us to pulse AS SOON AS we have enough images, rather than waiting for the entire batch.
		timeout := time.After(MaxImageWaitRetry * ImageWaitRetryDelay) // Fallback timeout

		for {
			select {
			case <-downloadDone:
				// All downloads finished. Pulse immediately found images.
				wp.actionChan <- func() {
					wp.imgPulseOp()
				}
				return
			case <-timeout:
				// Timed out waiting for images. Pulse anyway (might fail or show old/none).
				wp.actionChan <- func() {
					wp.imgPulseOp()
				}
				return
			case <-time.After(ImageWaitRetryDelay):
				// Check if we have enough images to proceed early
				wp.downloadMutex.Lock()
				count := len(wp.localImgRecs)
				wp.downloadMutex.Unlock()

				if count >= MinLocalImageBeforePulse {
					log.Debugf("Found %d images (>= %d), pulsing early.", count, MinLocalImageBeforePulse)
					// Pulse logic: Trigger next wallpaper via channel
					wp.actionChan <- func() {
						wp.imgPulseOp()
					}
					// We continue downloading in background, but we don't need to wait here anymore.
					// However, we should let the goroutine exit?
					// Actually, the downloadAllImages goroutine is ensuring cleanup.
					// We just exit THIS goroutine.
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

// SetShuffleImage enables or disables image shuffling.
func (wp *Plugin) SetShuffleImage(enabled bool) {
	wp.shuffleImageFlag.Set(enabled)
	wp.cfg.SetImgShuffle(enabled)

	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if wp.shuffleImageFlag.Value() {
		// Use internal method for the op
		wp.imgPulseOp = wp.setRandomWallpaper
		wp.manager.NotifyUser("Wallpaper Shuffling", "Enabled")
	} else {
		// Use internal method for the op
		wp.imgPulseOp = wp.setNextWallpaper
		wp.manager.NotifyUser("Wallpaper Shuffling", "Disabled")
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
