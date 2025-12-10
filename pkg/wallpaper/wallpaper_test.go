package wallpaper

import (
	"context"
	"image"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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

func (m *MockImageProcessor) FitImage(ctx context.Context, img interface{}) (interface{}, error) {
	args := m.Called(ctx, img)
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

func (m *MockImageProcessorTyped) FitImage(ctx context.Context, img image.Image) (image.Image, error) {
	args := m.Called(ctx, img)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(image.Image), args.Error(1)
}

// MockImageProvider implements ImageProvider for testing
type MockImageProvider struct {
	mock.Mock
}

func (m *MockImageProvider) Name() string {
	args := m.Called()
	return args.String(0)
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

func (m *MockImageProvider) Title() string   { return "Mock" }
func (m *MockImageProvider) HomeURL() string { return "https://mock.provider" }
func (m *MockImageProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}
func (m *MockImageProvider) CreateQueryPanel(sm setting.SettingsManager) fyne.CanvasObject {
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
	mockProvider.On("Name").Return("MockProvider")
	mockProvider.On("ParseURL", "http://mock.url").Return("http://api.mock.url", nil)

	// Mock FetchImages to return one image
	mockProvider.On("FetchImages", mock.Anything, "http://api.mock.url", 1).Return([]provider.Image{
		{
			ID:          "test_img_1",
			Path:        "http://example.com/image1.jpg", // We will mock this download
			ViewURL:     "http://whvn.cc/test_img_1",
			Attribution: "tester",
			Provider:    "MockProvider",
			FileType:    "image/jpeg",
		},
	}, nil)

	cfg.Queries = []ImageQuery{}
	_, err := cfg.AddImageQuery("Test Query", "http://mock.url", true)
	assert.NoError(t, err)

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)
	mockIP := new(MockImageProcessorTyped)

	// Mock HTTP Client to intercept image download
	// The provider returns "http://example.com/image1.jpg" as Path.
	// We need to intercept this.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image1.jpg" {
			_, _ = w.Write([]byte("fake image content"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	// Update the mock provider to return the ts URL for the image path
	mockProvider.ExpectedCalls = nil // Setup mock provider
	mockProvider.On("Name").Return("MockProvider")
	mockProvider.On("ParseURL", mock.Anything).Return("http://mock.url", nil)
	img := provider.Image{ID: "test_img_1", Path: ts.URL + "/image1.jpg", Provider: "MockProvider"}
	mockProvider.On("FetchImages", mock.Anything, "http://mock.url", 1).Return([]provider.Image{img}, nil)
	// Expect EnrichImage call
	mockProvider.On("EnrichImage", mock.Anything, mock.Anything).Return(img, nil)

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
		seenImages:          make(map[string]bool),
		providers:           make(map[string]provider.ImageProvider),
		localImgRecs:        []provider.Image{},
	}
	wp.providers["MockProvider"] = mockProvider

	// Expectations
	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	mockOS.On("getDesktopDimension").Return(1920, 1080, nil)

	// Run
	wp.downloadAllImages(nil)

	// Verify
	mockPM.AssertCalled(t, "NotifyUser", "Downloading: ", "[Test Query]\n")

	// Check if file exists in localImgRecs
	assert.Equal(t, 1, len(wp.localImgRecs))
	assert.Equal(t, "test_img_1", wp.localImgRecs[0].ID)
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
	mockProvider.On("ParseURL", mock.Anything).Return("http://api.mock.url", nil)
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
		seenImages:          make(map[string]bool),
		providers:           make(map[string]provider.ImageProvider),
		localImgRecs:        []provider.Image{},
	}
	wp.providers["MockProvider"] = mockProvider

	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	mockOS.On("getDesktopDimension").Return(1920, 1080, nil)

	// Run
	wp.downloadAllImages(nil)

	// Verify
	// Should still have downloaded the image
	assert.Equal(t, 1, len(wp.localImgRecs))
	assert.Equal(t, "test_img_fail", wp.localImgRecs[0].ID)
	// Attribution should remain original
	assert.Equal(t, "Original", wp.localImgRecs[0].Attribution)
}

func TestNavigation(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)
	mockIP := new(MockImageProcessorTyped)

	wp := &Plugin{
		os:                  mockOS,
		imgProcessor:        mockIP,
		cfg:                 cfg,
		manager:             mockPM,
		downloadedDir:       t.TempDir(),
		interrupt:           util.NewSafeBool(),
		currentDownloadPage: util.NewSafeIntWithValue(1),
		fitImageFlag:        util.NewSafeBool(),
		shuffleImageFlag:    util.NewSafeBool(),
		seenImages:          make(map[string]bool),
		localImgIndex:       *util.NewSafeIntWithValue(-1),
		actionChan:          make(chan func(), 10),
	}

	// Helper to execute queued actions synchronously for testing
	processActions := func() {
		close(wp.actionChan)
		for action := range wp.actionChan {
			action()
		}
		// Re-open for next steps? No, simpler to just loop with select or buffer check.
		// Better: process one item.
	}
	_ = processActions // Keep compiler happy if unused temporarily

	processQueue := func() {
		for {
			select {
			case action := <-wp.actionChan:
				action()
			default:
				return
			}
		}
	}

	// Setup initial state with some images
	img1 := provider.Image{ID: "img1", Path: "http://example.com/img1.jpg", FilePath: "path/to/img1.jpg", Attribution: "user1"}
	img2 := provider.Image{ID: "img2", Path: "http://example.com/img2.jpg", FilePath: "path/to/img2.jpg", Attribution: "user2"}
	wp.localImgRecs = []provider.Image{img1, img2}

	// Mock OS setWallpaper
	mockOS.On("setWallpaper", mock.Anything).Return(nil)

	// Expect NotifyUser when setting shuffle
	mockPM.On("NotifyUser", "Wallpaper Shuffling", "Disabled").Return()

	// Test SetNextWallpaper (Shuffle disabled)
	wp.SetShuffleImage(false)
	// SetShuffleImage sets imgPulseOp to setNextWallpaper

	// Initial state: index -1. Next should be 0.
	wp.SetNextWallpaper()
	processQueue()
	assert.Equal(t, 0, wp.localImgIndex.Value())
	assert.Equal(t, "img1", wp.currentImage.ID)
	mockOS.AssertCalled(t, "setWallpaper", mock.MatchedBy(func(path string) bool {
		return strings.HasSuffix(path, "img1.jpg")
	}))

	// Next should be 1
	wp.SetNextWallpaper()
	processQueue()
	assert.Equal(t, 1, wp.localImgIndex.Value())
	assert.Equal(t, "img2", wp.currentImage.ID)

	// Next should wrap to 0
	wp.SetNextWallpaper()
	processQueue()
	assert.Equal(t, 0, wp.localImgIndex.Value())
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
