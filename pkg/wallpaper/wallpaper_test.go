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
	"github.com/dixieflatline76/Spice/pkg/ui"
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

func TestDownloadAllImages(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Mock Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		// Return mock JSON response
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": [
				{
					"id": "test_img_1",
					"path": "http://example.com/image1.jpg",
					"thumbs": {"large": "http://example.com/thumb1.jpg"},
					"short_url": "http://whvn.cc/test_img_1"
				}
			]
		}`))
	}))
	defer ts.Close()

	// Add a query pointing to mock server
	// We need to override the URL in the query to point to ts.URL
	// But the query URL is usually "https://wallhaven.cc/..."
	// The code parses the URL.
	// We can add a query with ts.URL.

	// But `downloadImagesForURL` parses the query URL.
	// If we provide `ts.URL`, it will use it.

	cfg.ImageQueries = []ImageQuery{}
	_, err := cfg.AddImageQuery("Test Query", ts.URL, true)
	assert.NoError(t, err)

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)
	mockIP := new(MockImageProcessorTyped)

	// Create plugin instance manually to inject mocks
	wp := &wallpaperPlugin{
		os:           mockOS,
		imgProcessor: mockIP,
		cfg:          cfg,
		httpClient:   ts.Client(), // Use client that trusts the server (though httptest server is http usually)
		manager:      mockPM,
		// Initialize other fields
		downloadedDir:       t.TempDir(),
		interrupt:           util.NewSafeBool(),
		currentDownloadPage: util.NewSafeIntWithValue(1),
		fitImageFlag:        util.NewSafeBool(),
		shuffleImageFlag:    util.NewSafeBool(),
		seenImages:          make(map[string]bool),
	}

	// Expectations
	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()
	mockOS.On("getDesktopDimension").Return(1920, 1080, nil)

	// We need to handle the image download itself.
	// The response contains "path": "http://example.com/image1.jpg".
	// The code will try to download this URL.
	// "example.com" will fail or hit real network.
	// We should make the image path point to our mock server too.
	// But `downloadImagesForURL` parses the JSON response.
	// We can control the JSON response.
	// So we set "path": ts.URL + "/image1.jpg".

	// Update mock handler to serve image
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image1.jpg" {
			w.Write([]byte("fake image content"))
			return
		}
		// Default: serve JSON
		w.Header().Set("Content-Type", "application/json")
		// Use ts.URL in the response
		responseJSON := `{
			"data": [
				{
					"id": "test_img_1",
					"path": "` + ts.URL + `/image1.jpg",
					"thumbs": {"large": "http://example.com/thumb1.jpg"},
					"short_url": "http://whvn.cc/test_img_1"
				}
			]
		}`
		w.Write([]byte(responseJSON))
	})

	// Run
	wp.downloadAllImages()

	// Verify
	mockPM.AssertCalled(t, "NotifyUser", "Downloading: ", "[Test Query]\n")
	// Verify file exists
	// Filename is extracted from URL. "image1.jpg".
	// Path: wp.downloadedDir/image1.jpg
	// Wait, `extractFilenameFromURL` logic.
	// URL: ts.URL + "/image1.jpg".
	// It should extract "image1.jpg".

	// Check if file exists
	// We need to know the path.
	// But `downloadAllImages` runs goroutines.
	// It waits for them.

	// We can check `wp.localImgRecs`.
	assert.Equal(t, 1, len(wp.localImgRecs))
	assert.Equal(t, "test_img_1", wp.localImgRecs[0].ID)
}

func TestNavigation(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	mockOS := new(MockOS)
	mockPM := new(MockPluginManager)
	mockIP := new(MockImageProcessorTyped)

	wp := &wallpaperPlugin{
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
	}

	// Setup initial state with some images
	img1 := ImgSrvcImage{ID: "img1", Path: "http://example.com/img1.jpg"}
	img2 := ImgSrvcImage{ID: "img2", Path: "http://example.com/img2.jpg"}
	wp.localImgRecs = []ImgSrvcImage{img1, img2}

	// Mock OS setWallpaper
	mockOS.On("setWallpaper", mock.Anything).Return(nil)

	// Expect NotifyUser when setting shuffle
	mockPM.On("NotifyUser", "Wallpaper Shuffling", "Disabled").Return()

	// Test SetNextWallpaper (Shuffle disabled)
	wp.SetShuffleImage(false)
	// SetShuffleImage sets imgPulseOp to setNextWallpaper

	// Initial state: index -1. Next should be 0.
	wp.SetNextWallpaper()
	assert.Equal(t, 0, wp.localImgIndex.Value())
	assert.Equal(t, "img1", wp.currentImage.ID)
	mockOS.AssertCalled(t, "setWallpaper", mock.MatchedBy(func(path string) bool {
		return strings.HasSuffix(path, "img1.jpg")
	}))

	// Next should be 1
	wp.SetNextWallpaper()
	assert.Equal(t, 1, wp.localImgIndex.Value())
	assert.Equal(t, "img2", wp.currentImage.ID)

	// Next should wrap to 0
	wp.SetNextWallpaper()
	assert.Equal(t, 0, wp.localImgIndex.Value())
}
