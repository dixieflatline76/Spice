package wallpaper

import (
	"context"
	"image"
	"net/url"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
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

func (m *MockPluginManager) OpenURL(u *url.URL) error {
	args := m.Called(u)
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

// MockOS is a mock implementation of the OS interface.
type MockOS struct {
	mock.Mock
}

func (m *MockOS) GetDesktopDimension() (int, int, error) {
	args := m.Called()
	return args.Int(0), args.Int(1), args.Error(2)
}

func (m *MockOS) SetWallpaper(path string, monitorID int) error {
	args := m.Called(path, monitorID)
	return args.Error(0)
}

func (m *MockOS) GetMonitors() ([]Monitor, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Monitor), args.Error(1)
}

// MockImageStore is a mock implementation of the StoreInterface.
type MockImageStore struct {
	mock.Mock
}

func (m *MockImageStore) Count() int {
	args := m.Called()
	return args.Int(0)
}

func (m *MockImageStore) Get(index int) (provider.Image, bool) {
	args := m.Called(index)
	return args.Get(0).(provider.Image), args.Bool(1)
}

func (m *MockImageStore) Remove(id string) (provider.Image, bool) {
	args := m.Called(id)
	return args.Get(0).(provider.Image), args.Bool(1)
}

func (m *MockImageStore) MarkSeen(filePath string) {
	m.Called(filePath)
}

func (m *MockImageStore) SeenCount() int {
	args := m.Called()
	return args.Int(0)
}

// MockImageProcessor is a mock implementation of the ImageProcessor interface.
type MockImageProcessor struct {
	mock.Mock
}

func (m *MockImageProcessor) DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error) {
	args := m.Called(ctx, imgBytes, contentType)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).(image.Image), args.String(1), args.Error(2)
}

func (m *MockImageProcessor) EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error) {
	args := m.Called(ctx, img, contentType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockImageProcessor) FitImage(ctx context.Context, img image.Image, targetWidth, targetHeight int) (image.Image, error) {
	args := m.Called(ctx, img, targetWidth, targetHeight)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(image.Image), args.Error(1)
}

func (m *MockImageProcessor) CheckCompatibility(imgWidth, imgHeight, targetWidth, targetHeight int) error {
	args := m.Called(imgWidth, imgHeight, targetWidth, targetHeight)
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
	args := m.Called()
	if len(args) == 0 {
		return true
	}
	return args.Bool(0)
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
		return "Mock"
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
