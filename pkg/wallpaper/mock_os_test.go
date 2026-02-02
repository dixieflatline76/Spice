package wallpaper

import "github.com/stretchr/testify/mock"

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
