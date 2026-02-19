package wallpaper

import (
	"sync"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/mock"
)

type mockDeepDeleteFavoriter struct {
	mock.Mock
}

func (m *mockDeepDeleteFavoriter) AddFavorite(img provider.Image) error {
	args := m.Called(img)
	return args.Error(0)
}

func (m *mockDeepDeleteFavoriter) RemoveFavorite(img provider.Image) error {
	args := m.Called(img)
	return args.Error(0)
}

func (m *mockDeepDeleteFavoriter) IsFavorited(img provider.Image) bool {
	args := m.Called(img)
	return args.Bool(0)
}

func (m *mockDeepDeleteFavoriter) GetSourceQueryID() string {
	args := m.Called()
	return args.String(0)
}

func TestToggleFavorite_DeepDeleteLogic(t *testing.T) {
	t.Run("Unfavorite Local Favorite - Should Deep Delete", func(t *testing.T) {
		wp := &Plugin{
			favoriter:          &mockDeepDeleteFavoriter{},
			store:              &MockImageStore{},
			manager:            &mockManager{},
			cfg:                GetConfig(NewMockPreferences()),
			providers:          make(map[string]provider.ImageProvider),
			Monitors:           make(map[int]*MonitorController),
			fetchingInProgress: util.NewSafeBool(),
		}

		mf := wp.favoriter.(*mockDeepDeleteFavoriter)
		ms := wp.store.(*MockImageStore)
		mm := wp.manager.(*mockManager)

		img := provider.Image{
			ID:          "fav_123",
			Provider:    "Favorites",
			IsFavorited: true,
		}

		mf.On("RemoveFavorite", img).Return(nil)
		ms.On("Remove", img.ID).Return(img, true)
		mm.On("NotifyUser", mock.Anything, mock.Anything).Return()

		// Background telemetry calls should be ignored
		ms.On("SeenCount").Return(0).Maybe()
		ms.On("Count").Return(0).Maybe()

		wp.ToggleFavorite(img)

		mf.AssertExpectations(t)
		ms.AssertExpectations(t)
	})

	t.Run("Unfavorite Source Image - Should NOT Deep Delete", func(t *testing.T) {
		wp := &Plugin{
			favoriter:          &mockDeepDeleteFavoriter{},
			store:              &MockImageStore{},
			manager:            &mockManager{},
			cfg:                GetConfig(NewMockPreferences()),
			providers:          make(map[string]provider.ImageProvider),
			Monitors:           make(map[int]*MonitorController),
			fetchingInProgress: util.NewSafeBool(),
		}

		mf := wp.favoriter.(*mockDeepDeleteFavoriter)
		ms := wp.store.(*MockImageStore)
		mm := wp.manager.(*mockManager)

		img := provider.Image{
			ID:          "wallhaven_456",
			Provider:    "Wallhaven",
			IsFavorited: true,
		}

		mf.On("RemoveFavorite", img).Return(nil)
		ms.On("Update", mock.MatchedBy(func(i provider.Image) bool {
			return i.ID == img.ID && i.IsFavorited == false
		})).Return(true)
		mm.On("NotifyUser", mock.Anything, mock.Anything).Return()

		// Background telemetry calls should be ignored
		ms.On("SeenCount").Return(0).Maybe()
		ms.On("Count").Return(0).Maybe()

		wp.ToggleFavorite(img)

		mf.AssertExpectations(t)
		ms.AssertNotCalled(t, "Remove", mock.Anything)
	})

	t.Run("Favorite Source Image - Should Update Store", func(t *testing.T) {
		wp := &Plugin{
			favoriter:          &mockDeepDeleteFavoriter{},
			store:              &MockImageStore{},
			manager:            &mockManager{},
			cfg:                GetConfig(NewMockPreferences()),
			providers:          make(map[string]provider.ImageProvider),
			Monitors:           make(map[int]*MonitorController),
			downloadMutex:      sync.RWMutex{},
			queryPages:         make(map[string]*util.SafeCounter),
			fetchingInProgress: util.NewSafeBool(),
		}

		mf := wp.favoriter.(*mockDeepDeleteFavoriter)
		ms := wp.store.(*MockImageStore)
		mm := wp.manager.(*mockManager)

		img := provider.Image{
			ID:          "wallhaven_789",
			Provider:    "Wallhaven",
			IsFavorited: false,
		}

		mf.On("AddFavorite", img).Return(nil)
		ms.On("Update", mock.MatchedBy(func(i provider.Image) bool {
			return i.ID == img.ID && i.IsFavorited == true
		})).Return(true)
		mm.On("NotifyUser", mock.Anything, mock.Anything).Return()

		// Background telemetry calls should be ignored
		ms.On("SeenCount").Return(0).Maybe()
		ms.On("Count").Return(0).Maybe()

		wp.ToggleFavorite(img)

		mf.AssertExpectations(t)
		ms.AssertExpectations(t)
	})
}
