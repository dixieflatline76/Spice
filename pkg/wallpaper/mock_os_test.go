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

func (m *MockOS) SetWallpaper(path string) error {
	args := m.Called(path)
	return args.Error(0)
}
