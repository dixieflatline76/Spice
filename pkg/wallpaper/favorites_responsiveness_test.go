package wallpaper

import (
	"net/url"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockFavoriter struct {
	mock.Mock
}

func (m *mockFavoriter) AddFavorite(img provider.Image) error {
	args := m.Called(img)
	return args.Error(0)
}

func (m *mockFavoriter) RemoveFavorite(img provider.Image) error {
	args := m.Called(img)
	return args.Error(0)
}

func (m *mockFavoriter) IsFavorited(img provider.Image) bool {
	args := m.Called(img)
	return args.Bool(0)
}

func (m *mockFavoriter) GetSourceQueryID() string {
	args := m.Called()
	return args.String(0)
}

type mockManager struct {
	mock.Mock
}

func (m *mockManager) Register(p ui.Plugin) {
	m.Called(p)
}

func (m *mockManager) Deregister(p ui.Plugin) {
	m.Called(p)
}

func (m *mockManager) NotifyUser(title, message string) {
	m.Called(title, message)
}

func (m *mockManager) RegisterNotifier(n ui.Notifier) {
	m.Called(n)
}

func (m *mockManager) CreateMenuItem(label string, action func(), providerId string) *fyne.MenuItem {
	return nil
}

func (m *mockManager) CreateToggleMenuItem(label string, action func(bool), providerId string, val bool) *fyne.MenuItem {
	return nil
}

func (m *mockManager) OpenURL(u *url.URL) error {
	args := m.Called(u)
	return args.Error(0)
}

func (m *mockManager) OpenPreferences(name string) {
	m.Called(name)
}

func (m *mockManager) RebuildTrayMenu() {
	m.Called()
}

func (m *mockManager) RefreshTrayMenu() {
	m.Called()
}

func (m *mockManager) GetPreferences() fyne.Preferences {
	return nil
}

func (m *mockManager) GetAssetManager() *asset.Manager {
	return nil
}

func TestToggleFavorite_ResetsPageAndTriggersFetch(t *testing.T) {
	// Setup
	store := NewImageStore()
	mockMgr := &mockManager{}
	wp := &Plugin{
		store:              store,
		queryPages:         make(map[string]*util.SafeCounter),
		cfg:                &Config{},
		manager:            mockMgr,
		fetchingInProgress: util.NewSafeBool(),
	}

	mockFav := &mockFavoriter{}
	wp.favoriter = mockFav

	img := provider.Image{ID: "test_img", IsFavorited: false}

	// Set Favorites page to something other than 1
	wp.queryPages[FavoritesQueryID] = util.NewSafeIntWithValue(5)

	// Expectations
	mockFav.On("AddFavorite", img).Return(nil)
	mockMgr.On("NotifyUser", "Favorites", "Added to favorites.").Return()

	// Action
	wp.ToggleFavorite(img)

	// Verify
	assert.Equal(t, 1, wp.queryPages[FavoritesQueryID].Value(), "Page counter should be reset to 1")
}
