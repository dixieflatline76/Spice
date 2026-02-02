package wallpaper

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockPluginManager implements ui.PluginManager for testing
type MockPluginManager struct {
	mock.Mock
}

func (m *MockPluginManager) Register(p ui.Plugin) {
	m.Called(p)
}

func (m *MockPluginManager) Deregister(p ui.Plugin) {
	m.Called(p)
}

func (m *MockPluginManager) NotifyUser(title, message string) {
	m.Called(title, message)
}

func (m *MockPluginManager) RegisterNotifier(n ui.Notifier) {
	m.Called(n)
}

func (m *MockPluginManager) CreateMenuItem(label string, action func(), iconName string) *fyne.MenuItem {
	args := m.Called(label, action, iconName)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*fyne.MenuItem)
}

func (m *MockPluginManager) CreateToggleMenuItem(label string, action func(bool), iconName string, checked bool) *fyne.MenuItem {
	args := m.Called(label, action, iconName, checked)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*fyne.MenuItem)
}

func (m *MockPluginManager) OpenURL(url *url.URL) error {
	args := m.Called(url)
	return args.Error(0)
}

func (m *MockPluginManager) OpenPreferences(tab string) {
	m.Called(tab)
}

func (m *MockPluginManager) GetPreferences() fyne.Preferences {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(fyne.Preferences)
}

func (m *MockPluginManager) GetAssetManager() *asset.Manager {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*asset.Manager)
}

func (m *MockPluginManager) RefreshTrayMenu() {
	m.Called()
}

func (m *MockPluginManager) RebuildTrayMenu() {
	m.Called()
}

// MockImageProcessor implements ImageProcessor for testing
type MockImageProcessor struct {
	mock.Mock
}

func (m *MockImageProcessor) DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (interface{}, string, error) {
	// Return type is image.Image, but mock needs interface{}
	args := m.Called(ctx, imgBytes, contentType)
	return args.Get(0), args.String(1), args.Error(2)
}

// Helper to cast interface{} to image.Image if needed, but for now we just return nil or mock object
// Since image.Image is interface, it works.

func (m *MockImageProcessor) EncodeImage(ctx context.Context, img interface{}, contentType string) ([]byte, error) {
	args := m.Called(ctx, img, contentType)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockImageProcessor) FitImage(ctx context.Context, img interface{}, targetWidth, targetHeight int) (interface{}, error) {
	args := m.Called(ctx, img, targetWidth, targetHeight)
	return args.Get(0), args.Error(1)
}

// We need to adapt MockImageProcessor to match ImageProcessor interface which uses image.Image
// Since image.Image is an interface, we can use it directly in signature.

type MockImageProcessorTyped struct {
	mock.Mock
}

func (m *MockImageProcessorTyped) DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error) {
	args := m.Called(ctx, imgBytes, contentType)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).(image.Image), args.String(1), args.Error(2)
}

func (m *MockImageProcessorTyped) EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error) {
	args := m.Called(ctx, img, contentType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockImageProcessorTyped) FitImage(ctx context.Context, img image.Image, targetWidth, targetHeight int) (image.Image, error) {
	args := m.Called(ctx, img, targetWidth, targetHeight)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(image.Image), args.Error(1)
}

func (m *MockImageProcessorTyped) CheckCompatibility(width, height, targetWidth, targetHeight int) error {
	args := m.Called(width, height, targetWidth, targetHeight)
	return args.Error(0)
}

// MockImageProvider implements ImageProvider for testing
type MockImageProvider struct {
	mock.Mock
}

func (m *MockImageProvider) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockImageProvider) Type() provider.ProviderType {
	return provider.TypeOnline
}

func (m *MockImageProvider) SupportsUserQueries() bool {
	return true
}

func (m *MockImageProvider) ParseURL(webURL string) (string, error) {
	args := m.Called(webURL)
	return args.String(0), args.Error(1)
}

func (m *MockImageProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	args := m.Called(ctx, apiURL, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]provider.Image), args.Error(1)
}

func (m *MockImageProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	args := m.Called(ctx, img)
	if args.Get(0) == nil {
		return provider.Image{}, args.Error(1)
	}
	return args.Get(0).(provider.Image), args.Error(1)
}

func (m *MockImageProvider) Title() string {
	args := m.Called()
	if len(args) == 0 {
		return "Mock" // Fallback for existing tests that don't set expectation
	}
	return args.String(0)
}
func (m *MockImageProvider) HomeURL() string { return "https://mock.provider" }
func (m *MockImageProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}
func (m *MockImageProvider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	return nil
}
func (m *MockImageProvider) GetProviderIcon() fyne.Resource { return nil }

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
	mockIP := new(MockImageProcessorTyped)

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
	mockProvider.On("FetchImages", mock.Anything, "http://api.mock.url", 1).Return([]provider.Image{img}, nil)
	// Expect EnrichImage call
	mockProvider.On("EnrichImage", mock.Anything, mock.Anything).Return(img, nil)

	// Expect FitImage call if SmartFit is enabled
	mockIP.On("FitImage", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(imgRaw, nil)

	// Create plugin instance manually to inject mocks
	wp := &Plugin{
		os:           mockOS,
		imgProcessor: mockIP,
		cfg:          cfg,
		httpClient:   ts.Client(),
		manager:      mockPM,
		// Initialize other fields
		downloadedDir:       t.TempDir(),
		interrupt:           util.NewSafeBool(),
		currentDownloadPage: util.NewSafeIntWithValue(1),
		fitImageFlag:        util.NewSafeBool(),
		shuffleImageFlag:    util.NewSafeBool(),
		fetchingInProgress:  util.NewSafeBool(),
		providers:           make(map[string]provider.ImageProvider),
		store:               NewImageStore(),
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
	// Verify
	// Allow extra queries in message due to config defaults leakage in tests
	mockPM.AssertCalled(t, "NotifyUser", "Downloading: ", mock.Anything)

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
	mockIP := new(MockImageProcessorTyped)

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
		os:                  mockOS,
		imgProcessor:        mockIP,
		cfg:                 cfg,
		httpClient:          ts.Client(),
		manager:             mockPM,
		downloadedDir:       t.TempDir(),
		interrupt:           util.NewSafeBool(),
		currentDownloadPage: util.NewSafeIntWithValue(1),
		fitImageFlag:        util.NewSafeBool(),
		shuffleImageFlag:    util.NewSafeBool(),
		fetchingInProgress:  util.NewSafeBool(),
		providers:           make(map[string]provider.ImageProvider),
		store:               NewImageStore(),
		runOnUI:             func(f func()) { f() }, // Run synchronously in tests
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

	img1 := provider.Image{ID: "img1", Path: "http://example.com/img1.jpg", FilePath: img1Path, Attribution: "user1"}
	img2 := provider.Image{ID: "img2", Path: "http://example.com/img2.jpg", FilePath: img2Path, Attribution: "user2"}

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
	wp.providerMenuItem = &fyne.MenuItem{}
	wp.artistMenuItem = &fyne.MenuItem{}

	// Create Monitor Controller
	mc := NewMonitorController(0, Monitor{ID: 0}, wp.store, wp.fm, mockOS, cfg)
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
	wp.SetNextWallpaper()
	pump()

	// Verify
	assert.Equal(t, "img1", mc.State.CurrentImage.ID)
	mockOS.AssertCalled(t, "SetWallpaper", mock.MatchedBy(func(path string) bool {
		return strings.HasSuffix(path, "img1.jpg")
	}), 0)

	// 3. Next -> img2
	wp.SetNextWallpaper()
	pump()
	assert.Equal(t, "img2", mc.State.CurrentImage.ID)

	// 4. Next -> img1 (wrap)
	wp.SetNextWallpaper()
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
	mockIP := new(MockImageProcessorTyped)
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
	assert.Equal(t, masterPath, path)

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
	mc := NewMonitorController(0, Monitor{ID: 0}, wp.store, wp.fm, mockOS, cfg)
	mc.State.CurrentImage = img
	wp.Monitors[0] = mc

	// Mock OS for setWallpaper
	mockOS.On("SetWallpaper", mock.Anything, 0).Return(nil)

	// Execute Delete (Async dispatch)
	wp.DeleteCurrentImage()

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
	assert.Contains(t, err.Error(), "URL not supported")
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
	assert.Contains(t, err.Error(), "URL not supported")
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
