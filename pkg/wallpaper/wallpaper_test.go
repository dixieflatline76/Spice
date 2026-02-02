package wallpaper

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mocks removed: MockPluginManager

// Mocks removed: MockImageProcessor, MockImageProvider

func TestDownloadAllImages(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Mock Provider
	mockProvider := new(MockImageProvider)
	mockProvider.On("Name").Return("Wallhaven") // Pretend to be Wallhaven for default AddImageQuery
	mockProvider.On("ParseURL", "http://mock.url").Return("http://api.mock.url", nil)
	mockProvider.On("ParseURL", mock.Anything).Return("", assert.AnError)

	// Mock FetchImages to return one image
	mockProvider.On("FetchImages", mock.Anything, "http://api.mock.url", 1).Return([]provider.Image{
		{
			ID:          "test_img_1",
			Path:        "http://example.com/image1.jpg", // We will mock this download
			ViewURL:     "http://whvn.cc/test_img_1",
			Attribution: "tester",
			Provider:    "Wallhaven",
			FileType:    "image/jpeg",
		},
	}, nil)

	cfg.Queries = []ImageQuery{}
	_, err := cfg.AddImageQuery("Test Query", "http://mock.url", true)
	assert.NoError(t, err)

	// Create valid JPEG for testing
	var buf bytes.Buffer
	imgRaw := image.NewRGBA(image.Rect(0, 0, 1, 1))
	imgRaw.Set(0, 0, color.RGBA{255, 0, 0, 255})
	_ = jpeg.Encode(&buf, imgRaw, nil)
	validJPEG := buf.Bytes()

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)
	mockIP := new(MockImageProcessor)

	// Mock HTTP Client to intercept image download
	// The provider returns "http://example.com/image1.jpg" as Path.
	// We need to intercept this.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image1.jpg" {
			_, _ = w.Write(validJPEG)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	// Update the mock provider to return the ts URL for the image path
	mockProvider.ExpectedCalls = nil // Setup mock provider
	mockProvider.On("Name").Return("MockProvider")
	// ParseURL with Specific Match (Success)
	mockProvider.On("ParseURL", "http://mock.url").Return("http://api.mock.url", nil)
	// ParseURL with Catch-All (Error) - MUST BE DEFINED AFTER SPECIFIC
	mockProvider.On("ParseURL", mock.Anything).Return("", assert.AnError)
	img := provider.Image{ID: "test_img_1", Path: ts.URL + "/image1.jpg", Provider: "MockProvider"}
	mockProvider.On("FetchImages", mock.Anything, "http://mock.url", 1).Return([]provider.Image{img}, nil)
	// Expect EnrichImage call
	mockProvider.On("EnrichImage", mock.Anything, mock.Anything).Return(img, nil)

	// Expect FitImage call if SmartFit is enabled
	mockIP.On("FitImage", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(imgRaw, nil)
	mockIP.On("CheckCompatibility", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Create plugin instance manually to inject mocks
	wp := &Plugin{
		os:           mockOS,
		imgProcessor: mockIP,
		cfg:          cfg,
		httpClient:   ts.Client(),
		manager:      mockPM,
		// Initialize other fields
		downloadedDir:      t.TempDir(),
		interrupt:          util.NewSafeBool(),
		queryPages:         make(map[string]*util.SafeCounter),
		fitImageFlag:       util.NewSafeBool(),
		shuffleImageFlag:   util.NewSafeBool(),
		fetchingInProgress: util.NewSafeBool(),
		providers:          make(map[string]provider.ImageProvider),
		store:              NewImageStore(),
	}
	wp.store.SetAsyncSave(false)
	// Setup FileManager
	wp.fm = NewFileManager(wp.downloadedDir)
	assert.NoError(t, wp.fm.EnsureDirs())
	wp.store.SetFileManager(wp.fm, wp.downloadedDir+"/cache.json")
	wp.pipeline = NewPipeline(wp.cfg, wp.store, wp.ProcessImageJob)
	wp.pipeline.Start(1)
	defer wp.pipeline.Stop()
	wp.providers["Wallhaven"] = mockProvider

	// Expectations
	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)
	mockOS.On("GetMonitors").Return([]Monitor{{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)}}, nil)

	// Run
	wp.FetchNewImages()

	// Verify

	// Check if file exists in store (wait for pipeline)
	assert.Eventually(t, func() bool {
		return wp.store.Count() == 1
	}, 2*time.Second, 100*time.Millisecond)

	imgStored, ok := wp.store.Get(0)
	assert.True(t, ok)
	assert.Equal(t, "test_img_1", imgStored.ID)
}

func TestDownloadAllImages_EnrichmentFailure(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Mock Provider
	mockProvider := &MockImageProvider{}
	mockProvider.On("Name").Return("Mock")
	mockProvider.On("Title").Return("Mock Provider")
	mockProvider.On("CreateSettingsPanel", mock.Anything).Return(nil)
	mockProvider.On("CreateQueryPanel", mock.Anything).Return(nil)

	// We need to register it. But providers are registered in init().
	// We can manually add it to the map for testing if we access the plugin instance.
	// Setup test plugin
	wp := &Plugin{
		providers: make(map[string]provider.ImageProvider),
	}
	wp.providers["Mock"] = mockProvider

	// Test logic that uses providers...
	// For now just asserting the interface compatibility
	var _ provider.ImageProvider = mockProvider

	// Mock FetchImages to return one image
	img := provider.Image{
		ID:          "test_img_fail",
		Path:        "http://example.com/image_fail.jpg",
		Provider:    "Mock",
		Attribution: "Original",
	}
	mockProvider.On("ParseURL", "http://mock.url").Return("http://api.mock.url", nil)
	mockProvider.On("FetchImages", mock.Anything, "http://api.mock.url", 1).Return([]provider.Image{img}, nil)

	// Expect EnrichImage call to FAIL
	mockProvider.On("EnrichImage", mock.Anything, mock.Anything).Return(provider.Image{}, assert.AnError)

	cfg.Queries = []ImageQuery{}
	_, err := cfg.AddImageQuery("Test Query", "http://mock.url", true)
	assert.NoError(t, err)

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)
	mockIP := new(MockImageProcessor)

	// Mock HTTP Client
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image_fail.jpg" {
			_, _ = w.Write([]byte("fake image content"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	// Update image path to use mock server
	img.Path = ts.URL + "/image_fail.jpg"
	// Re-setup FetchImages with correct path
	mockProvider.ExpectedCalls = nil
	mockProvider.On("Name").Return("MockProvider")
	mockProvider.On("Name").Return("MockProvider")
	// The producer now iterates providers and calls ParseURL, so we must expect it implicitly or explicitly.
	// Since we iterate, it might be called with any typical URL.
	// But in this test, we call produceJobsForURL explicitly? No, downloadAllImages calls it.
	// We expect ParseURL with "http://mock.url" (the query URL).
	// ParseURL with Specific Match (Success)
	mockProvider.On("ParseURL", "http://mock.url").Return("http://api.mock.url", nil)
	// ParseURL with Catch-All (Error)
	mockProvider.On("ParseURL", mock.Anything).Return("", assert.AnError)
	mockProvider.On("FetchImages", mock.Anything, "http://api.mock.url", 1).Return([]provider.Image{img}, nil)
	mockProvider.On("EnrichImage", mock.Anything, mock.Anything).Return(provider.Image{}, assert.AnError)

	wp = &Plugin{
		os:                 mockOS,
		imgProcessor:       mockIP,
		cfg:                cfg,
		httpClient:         ts.Client(),
		manager:            mockPM,
		downloadedDir:      t.TempDir(),
		interrupt:          util.NewSafeBool(),
		queryPages:         make(map[string]*util.SafeCounter),
		fitImageFlag:       util.NewSafeBool(),
		shuffleImageFlag:   util.NewSafeBool(),
		fetchingInProgress: util.NewSafeBool(),
		providers:          make(map[string]provider.ImageProvider),
		store:              NewImageStore(),
		runOnUI:            func(f func()) { f() }, // Run synchronously in tests
	}
	wp.store.SetAsyncSave(false)
	// Setup FileManager
	wp.fm = NewFileManager(wp.downloadedDir)
	assert.NoError(t, wp.fm.EnsureDirs())
	wp.store.SetFileManager(wp.fm, wp.downloadedDir+"/cache.json")
	wp.pipeline = NewPipeline(wp.cfg, wp.store, wp.ProcessImageJob)
	wp.pipeline.Start(1)
	defer wp.pipeline.Stop()
	wp.providers["MockProvider"] = mockProvider

	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)
	mockOS.On("GetMonitors").Return([]Monitor{{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)}}, nil)

	// Run
	wp.FetchNewImages()

	// Verify
	// Strict Mode: Enrichment Failure -> Image Dropped -> Count 0
	// We wait a bit to ensure pipeline processed it
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, 0, wp.store.Count())
}

func TestNavigation(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)

	wp := &Plugin{
		os:            mockOS,
		cfg:           cfg,
		manager:       mockPM,
		downloadedDir: t.TempDir(),
		store:         NewImageStore(),
		Monitors:      make(map[int]*MonitorController),
		runOnUI:       func(f func()) { f() },
	}
	wp.store.SetAsyncSave(false)
	// Setup FileManager
	wp.fm = NewFileManager(wp.downloadedDir)
	assert.NoError(t, wp.fm.EnsureDirs())
	wp.store.SetFileManager(wp.fm, wp.downloadedDir+"/cache.json")

	// Setup Images
	tempDir := t.TempDir()
	img1Path := filepath.Join(tempDir, "img1.jpg")
	img2Path := filepath.Join(tempDir, "img2.jpg")
	assert.NoError(t, os.WriteFile(img1Path, []byte("dummy"), 0644))
	assert.NoError(t, os.WriteFile(img2Path, []byte("dummy"), 0644))

	img1 := provider.Image{
		ID:              "img1",
		Path:            "http://example.com/img1.jpg",
		FilePath:        img1Path,
		Attribution:     "user1",
		DerivativePaths: map[string]string{"1920x1080": "img1.jpg"},
	}
	img2 := provider.Image{
		ID:              "img2",
		Path:            "http://example.com/img2.jpg",
		FilePath:        img2Path,
		Attribution:     "user2",
		DerivativePaths: map[string]string{"1920x1080": "img2.jpg"},
	}

	wp.store.Add(img1)
	wp.store.Add(img2)

	// Mock OS
	mockOS.On("GetMonitors").Return([]Monitor{{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)}}, nil)
	mockOS.On("SetWallpaper", mock.Anything, 0).Return(nil)

	// Mock PM
	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	mockPM.On("RefreshTrayMenu").Return()
	mockPM.On("GetAssetManager").Return(&asset.Manager{})

	// Init UI
	wp.monitorMenu = make(map[int]*MonitorMenuItems)
	wp.monitorMenu[0] = &MonitorMenuItems{
		ProviderMenuItem: &fyne.MenuItem{},
		ArtistMenuItem:   &fyne.MenuItem{},
	}

	// Create Monitor Controller
	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(0, Monitor{ID: 0, Rect: image.Rect(0, 0, 1920, 1080)}, wp.store, wp.fm, mockOS, cfg, mockIP)
	wp.Monitors[0] = mc

	// Helper to pump
	pump := func() {
		select {
		case cmd := <-mc.Commands:
			mc.handleCommand(cmd)
		case <-time.After(100 * time.Millisecond):
			// Might be no command if logic skipped
		}
	}

	// 1. Set Shuffle False
	wp.SetShuffleImage(false)

	// 2. Next
	wp.SetNextWallpaper(-1)
	pump()

	// Verify
	assert.Equal(t, "img1", mc.State.CurrentImage.ID)
	mockOS.AssertCalled(t, "SetWallpaper", mock.MatchedBy(func(path string) bool {
		return strings.HasSuffix(path, "img1.jpg")
	}), 0)

	// 3. Next -> img2
	wp.SetNextWallpaper(-1)
	pump()
	assert.Equal(t, "img2", mc.State.CurrentImage.ID)

	// 4. Next -> img1 (wrap)
	wp.SetNextWallpaper(-1)
	pump()
	assert.Equal(t, "img1", mc.State.CurrentImage.ID)
}

func TestTogglePause(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	mockPM := new(MockPluginManager)

	wp := &Plugin{
		cfg:     cfg,
		manager: mockPM,
	}

	// Mock NotifyUser for frequency change
	mockPM.On("NotifyUser", "Wallpaper Change", mock.Anything).Return()

	// Initial state: Default frequency (Hourly)
	assert.Equal(t, FrequencyHourly, wp.cfg.GetWallpaperChangeFrequency())
	assert.False(t, wp.IsPaused())

	// Toggle Pause -> Should become Never
	wp.TogglePause()
	assert.Equal(t, FrequencyNever, wp.cfg.GetWallpaperChangeFrequency())
	assert.True(t, wp.IsPaused())
	assert.Equal(t, FrequencyHourly, wp.prePauseFrequency) // Should store previous

	// Toggle Resume -> Should restore Hourly
	wp.TogglePause()
	assert.Equal(t, FrequencyHourly, wp.cfg.GetWallpaperChangeFrequency())
	assert.False(t, wp.IsPaused())

	// Test Resume with no history (simulate fresh start paused)
	wp.prePauseFrequency = FrequencyNever // Reset history
	wp.cfg.SetWallpaperChangeFrequency(FrequencyNever)
	wp.TogglePause()
	assert.Equal(t, FrequencyHourly, wp.cfg.GetWallpaperChangeFrequency()) // Should default to Hourly
}

func TestGetInstance(t *testing.T) {
	// Ensure singleton behavior
	instance1 := GetInstance()
	instance2 := GetInstance()
	assert.NotNil(t, instance1)
	assert.Equal(t, instance1, instance2)
}

func TestSmartFitDisabled(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	mockIP := new(MockImageProcessor)
	mockStore := NewImageStore()

	wp := &Plugin{
		cfg:           cfg,
		store:         mockStore,
		downloadedDir: t.TempDir(),
	}

	// 1. Set SmartFit to OFF (Disabled)
	cfg.SetSmartFitMode(SmartFitOff)
	// Also set FaceCrop/Boost to TRUE to ensure SmartFit overrides them
	cfg.SetFaceCropEnabled(true)
	cfg.SetFaceBoostEnabled(true)

	// 2. Prepare dummy master image
	masterPath := filepath.Join(wp.downloadedDir, "test_master.jpg")
	err := os.WriteFile(masterPath, []byte("dummy data"), 0644)
	assert.NoError(t, err)

	img := provider.Image{ID: "test_img"}

	// 3. Call ensureDerivative
	// It should return masterPath and err=nil.
	// It should NOT call FitImage.
	path, err := wp.ensureDerivative(context.Background(), img, masterPath)
	assert.NoError(t, err)
	assert.Equal(t, masterPath, path["primary"])

	// Verify FitImage was NEVER called
	mockIP.AssertNotCalled(t, "FitImage", mock.Anything, mock.Anything)
}

func TestDeleteCurrentImage_PersistsBlock(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Create mock plugin
	mockPM := new(MockPluginManager)
	mockOS := new(MockOS)

	wp := &Plugin{
		os:            mockOS,
		cfg:           cfg,
		manager:       mockPM,
		downloadedDir: t.TempDir(),
		store:         NewImageStore(),
		runOnUI:       func(f func()) { f() },
		Monitors:      make(map[int]*MonitorController),
	}
	wp.store.SetAsyncSave(false)
	// Setup FileManager
	wp.fm = NewFileManager(wp.downloadedDir)
	assert.NoError(t, wp.fm.EnsureDirs())
	wp.store.SetFileManager(wp.fm, wp.downloadedDir+"/cache.json")

	// Create a dummy image file to delete
	imgID := "delete_me"
	imgPath := filepath.Join(wp.downloadedDir, "delete_me.jpg")
	assert.NoError(t, os.WriteFile(imgPath, []byte("content"), 0644))

	img := provider.Image{
		ID:       imgID,
		FilePath: imgPath,
	}

	wp.store.Add(img)

	// Create Monitor Controller
	mockIP := new(MockImageProcessor)
	mc := NewMonitorController(0, Monitor{ID: 0}, wp.store, wp.fm, mockOS, cfg, mockIP)
	mc.State.CurrentImage = img
	wp.Monitors[0] = mc

	// Mock OS for setWallpaper
	mockOS.On("SetWallpaper", mock.Anything, 0).Return(nil)

	// Execute Delete (Async dispatch)
	wp.DeleteCurrentImage(0)

	// Manually pump the command
	select {
	case cmd := <-mc.Commands:
		assert.Equal(t, CmdDelete, cmd)
		mc.handleCommand(cmd)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for delete command")
	}

	// Verify AvoidSet
	assert.True(t, wp.cfg.InAvoidSet(imgID))

	// Verify Store Removal
	assert.Equal(t, 0, wp.store.Count())
}

func TestBlockFlow_PreventsReDownload(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Create mock plugin
	wp := &Plugin{
		cfg:           cfg,
		downloadedDir: t.TempDir(),
	}

	// 1. Block an image in Config
	blockedID := "blocked_img"
	cfg.AddToAvoidSet(blockedID)

	// 2. Create a Job for this image
	job := DownloadJob{
		Image: provider.Image{ID: blockedID, Path: "http://example.com/blocked.jpg"},
	}

	// 3. Attempt to Process
	_, err := wp.ProcessImageJob(context.Background(), job)

	// 4. Verify Rejection
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is in avoid set")
}

func TestOpenAddCollectionUI(t *testing.T) {
	// Setup
	mockPM := new(MockPluginManager)
	mockProvider := new(MockImageProvider)
	wp := &Plugin{
		manager:   mockPM,
		providers: make(map[string]provider.ImageProvider),
	}

	testURL := "https://wallhaven.cc/w/12345"
	wp.providers["Wallhaven"] = mockProvider

	// Expectation: ParseURL
	mockProvider.On("ParseURL", testURL).Return(testURL, nil)

	// Expectation: OpenPreferences
	mockProvider.On("SupportsUserQueries").Return(true)
	mockPM.On("OpenPreferences", "Wallpaper").Return()

	// Execute
	err := wp.OpenAddCollectionUI(testURL)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, testURL, wp.pendingAddUrl, "pendingAddUrl should be set")
	mockPM.AssertExpectations(t)
	mockProvider.AssertExpectations(t)
}

func TestOpenAddCollectionUI_InvalidProvider(t *testing.T) {
	// Setup
	mockPM := new(MockPluginManager)
	wp := &Plugin{
		manager:   mockPM,
		providers: make(map[string]provider.ImageProvider),
	}

	// No providers registered (or none match)
	testURL := "https://unknown.com/w/12345"

	// Execute
	err := wp.OpenAddCollectionUI(testURL)

	// Verify
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider found that can handle this URL")
	assert.Empty(t, wp.pendingAddUrl)
	mockPM.AssertNotCalled(t, "OpenPreferences")
}

type CuratedMockProvider struct {
	MockImageProvider
}

func (m *CuratedMockProvider) SupportsUserQueries() bool {
	return false
}

func TestOpenAddCollectionUI_CuratedSkipped(t *testing.T) {
	// Setup
	mockPM := new(MockPluginManager)
	mockProvider := new(CuratedMockProvider)
	wp := &Plugin{
		manager:   mockPM,
		providers: make(map[string]provider.ImageProvider),
	}

	testURL := "https://museum.org/collection/123"
	wp.providers["Museum"] = mockProvider

	// Expectation: ParseURL should NOT be called because SupportsUserQueries() checks first
	// Actually, wait. SupportsUserQueries() is called on the interface.
	// If SupportsUserQueries returns false, loop continues.
	// So ParseURL is NEVER called.

	// Execute
	err := wp.OpenAddCollectionUI(testURL)

	// Verify
	assert.Error(t, err) // Should error "URL not supported"
	assert.Contains(t, err.Error(), "no provider found that can handle this URL")
	mockProvider.AssertNotCalled(t, "ParseURL", mock.Anything)
	mockPM.AssertNotCalled(t, "OpenPreferences")
}

func TestGetProviderTitle(t *testing.T) {
	// Setup
	mockProvider := new(MockImageProvider)
	mockProvider.On("Title").Return("Mock Provider Title")

	wp := &Plugin{
		providers: make(map[string]provider.ImageProvider),
	}
	wp.providers["Mock"] = mockProvider

	// 1. Registered Provider -> Return Title
	assert.Equal(t, "Mock Provider Title", wp.GetProviderTitle("Mock"))

	// 2. Unregistered Provider -> Return ID
	assert.Equal(t, "UnknownProvider", wp.GetProviderTitle("UnknownProvider"))
}
func TestAddQuery_InitializesPage(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	mockPM := new(MockPluginManager)
	mockOS := new(MockOS)

	wp := &Plugin{
		os:                 mockOS,
		cfg:                cfg,
		manager:            mockPM,
		downloadedDir:      t.TempDir(),
		store:              NewImageStore(),
		queryPages:         make(map[string]*util.SafeCounter),
		fetchingInProgress: util.NewSafeBool(),
		providers:          make(map[string]provider.ImageProvider),
		pipeline:           NewPipeline(cfg, NewImageStore(), func(ctx context.Context, job DownloadJob) (provider.Image, error) { return job.Image, nil }),
		httpClient:         &http.Client{},
	}
	wp.fm = NewFileManager(wp.downloadedDir)

	// Add a provider
	mockProvider := &MockImageProvider{}
	mockProvider.On("Name").Return("Mock")
	mockProvider.On("FetchImages", mock.Anything, mock.Anything, 1).Return([]provider.Image{}, nil)
	wp.providers["Mock"] = mockProvider

	// Act: Add a query directly to config (simulating AddProviderQuery)
	cfg.Queries = []ImageQuery{{
		ID:       "new_query",
		Provider: "Mock",
		Active:   true,
		URL:      "http://mock",
	}}

	// Trigger Fetch
	wp.FetchNewImages()

	// Wait for fetch to complete
	assert.Eventually(t, func() bool {
		wp.downloadMutex.RLock()
		defer wp.downloadMutex.RUnlock()
		_, ok := wp.queryPages["new_query"]
		return ok
	}, 2*time.Second, 100*time.Millisecond)

	// Assert: Page counter should be initialized to 1
	wp.downloadMutex.Lock()
	pg, ok := wp.queryPages["new_query"]
	wp.downloadMutex.Unlock()

	assert.True(t, ok, "Page counter should exist for new query")
	if ok {
		assert.Equal(t, 1, pg.Value(), "Page counter should start at 1")
	}
}

func TestSetNextWallpaper_Stagger(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	wp := &Plugin{
		cfg:                cfg,
		Monitors:           make(map[int]*MonitorController),
		downloadMutex:      sync.RWMutex{},
		fitImageFlag:       util.NewSafeBool(),
		fetchingInProgress: util.NewSafeBool(),
	}

	// Setup 2 Monitors with Channels
	mon0 := &MonitorController{
		ID:       0,
		Commands: make(chan Command, 10),
	}
	mon1 := &MonitorController{
		ID:       1,
		Commands: make(chan Command, 10),
	}
	wp.Monitors[0] = mon0
	wp.Monitors[1] = mon1

	// Initialize monMu
	wp.monMu = sync.RWMutex{}

	// Enable Stagger
	cfg.SetStaggerMonitorChanges(true)

	// Set Frequency to Hourly (long duration)
	cfg.SetWallpaperChangeFrequency(FrequencyHourly)

	// Case 1: Stagger ON
	wp.SetNextWallpaper(-1)

	// Check Mon 0 (Immediate)
	select {
	case cmd := <-mon0.Commands:
		assert.Equal(t, CmdNext, cmd, "Monitor 0 should receive CmdNext immediately")
	default:
		t.Error("Monitor 0 did not receive command immediately")
	}

	// Check Mon 1 (Delayed)
	select {
	case <-mon1.Commands:
		t.Error("Monitor 1 received command immediately but should have been staggered")
	default:
		// Success: Channel empty
	}

	// Case 2: Stagger OFF
	cfg.SetStaggerMonitorChanges(false)

	// Drain Mon 0 just in case
	select {
	case <-mon0.Commands:
	default:
	}

	wp.SetNextWallpaper(-1)

	// Check Mon 0
	select {
	case cmd := <-mon0.Commands:
		assert.Equal(t, CmdNext, cmd)
	default:
		t.Error("Monitor 0 missing command")
	}

	// Check Mon 1 (Immediate now)
	select {
	case cmd := <-mon1.Commands:
		assert.Equal(t, CmdNext, cmd, "Monitor 1 should receive CmdNext immediately when Stagger is OFF")
	default:
		t.Error("Monitor 1 missing command when Stagger is OFF")
	}
}
