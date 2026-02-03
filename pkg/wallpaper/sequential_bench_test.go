package wallpaper

import (
	"fmt"
	"image"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"context"
	"net/url"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/mock"
)

// BenchMockPluginManager
type BenchMockPluginManager struct {
	mock.Mock
}

func (m *BenchMockPluginManager) Register(p ui.Plugin)           {}
func (m *BenchMockPluginManager) Deregister(p ui.Plugin)         {}
func (m *BenchMockPluginManager) NotifyUser(t, msg string)       {}
func (m *BenchMockPluginManager) RegisterNotifier(n ui.Notifier) {}
func (m *BenchMockPluginManager) CreateMenuItem(l string, a func(), i string) *fyne.MenuItem {
	return &fyne.MenuItem{}
}
func (m *BenchMockPluginManager) CreateToggleMenuItem(l string, a func(bool), i string, c bool) *fyne.MenuItem {
	return &fyne.MenuItem{}
}
func (m *BenchMockPluginManager) OpenURL(u *url.URL) error         { return nil }
func (m *BenchMockPluginManager) OpenPreferences(tab string)       {}
func (m *BenchMockPluginManager) GetPreferences() fyne.Preferences { return nil }
func (m *BenchMockPluginManager) GetAssetManager() *asset.Manager  { return nil }
func (m *BenchMockPluginManager) RefreshTrayMenu()                 {}
func (m *BenchMockPluginManager) RebuildTrayMenu()                 {}

// BenchMockProvider
type BenchMockProvider struct {
	mock.Mock
}

func (m *BenchMockProvider) Name() string                        { return "Wallhaven" }
func (m *BenchMockProvider) ParseURL(url string) (string, error) { return url, nil }
func (m *BenchMockProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	return nil, nil
}
func (m *BenchMockProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("dummy", []byte("dummy"))
}
func (m *BenchMockProvider) Title() string   { return "Wallhaven" }
func (m *BenchMockProvider) HomeURL() string { return "" }
func (m *BenchMockProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}
func (m *BenchMockProvider) Type() provider.ProviderType {
	return provider.TypeOnline
}

func (m *BenchMockProvider) SupportsUserQueries() bool {
	return true
}
func (m *BenchMockProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}
func (m *BenchMockProvider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	return nil
}
func (m *BenchMockProvider) GetDownloadHeaders() map[string]string { return nil }

// MockOS for benchmarking
type BenchMockOS struct {
	mock.Mock
}

func (m *BenchMockOS) SetWallpaper(path string, monitorID int) error {
	// Simulate OS syscall overhead (small)
	time.Sleep(10 * time.Millisecond)
	return nil
}

func (m *BenchMockOS) GetDesktopDimension() (int, int, error) {
	return 1920, 1080, nil
}

func (m *BenchMockOS) GetMonitors() ([]Monitor, error) {
	return []Monitor{{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)}}, nil
}

// BenchmarkSequentialSwitch measures the time to switch wallpapers sequentially.
// This mimics the user spamming "Next".
func BenchmarkSequentialSwitch(b *testing.B) {
	// 1. Setup
	tmpDir := b.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(true) // Enable the feature we just added
	store.SetDebounceDuration(1 * time.Second)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	// Create Plugin (Minimal)
	wp := &Plugin{
		manager:          &BenchMockPluginManager{},
		store:            store,
		os:               &BenchMockOS{},
		downloadMutex:    sync.RWMutex{},
		shuffleImageFlag: util.NewSafeBoolWithValue(false),
		fitImageFlag:     util.NewSafeBoolWithValue(false),
		// We need to mock "runOnUI" since we are in a test
		runOnUI: func(f func()) { f() },

		// Important: Initialize providers map
		providers: map[string]provider.ImageProvider{
			"Wallhaven": &BenchMockProvider{},
		},
	}

	// 2. Populate Store with Wallhaven-like images
	// Wallhaven images usually differ by not having attribution initially?
	// Or maybe they do. Let's assume typical state.
	count := 100
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("wh_img_%d", i)
		store.Add(provider.Image{
			ID:          id,
			Provider:    "Wallhaven",
			FilePath:    filepath.Join(tmpDir, id+".jpg"),
			Attribution: "", // Missing attribution triggers lookup?
		})
	}

	// 3. Benchmark Loop
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// We simulate "Next" action
		// In real app: wp.SetNextWallpaper()
		// We need to make sure wp.currentIndex rotates

		// Logic from SetNextWallpaper (simplified to avoid private method access issues if needed,
		// but since we are in package wallpaper, we can call it if we export it or use test hook)
		// SetNextWallpaper is private `func (wp *Plugin) SetNextWallpaper()`
		// We are in `wallpaper` package, so we CAN call it.

		wp.SetNextWallpaper(-1, true)
	}
}
