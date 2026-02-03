package main

import (
	"context"
	"fmt"
	"image"
	"os"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/stretchr/testify/mock"
)

// MockOS simulates the OS dependency
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

func (m *MockOS) GetCacheDir() (string, error) {
	return "test_cache", nil
}

func (m *MockOS) GetMonitors() ([]wallpaper.Monitor, error) {
	width, height, err := m.GetDesktopDimension()
	if err != nil {
		return nil, err
	}
	return []wallpaper.Monitor{{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, width, height)}}, nil
}

// MockPreferences simulates config
type MockPreferences struct {
	mock.Mock
}

func (m *MockPreferences) Bool(key string) bool                            { return true }
func (m *MockPreferences) Int(key string) int                              { return 0 }
func (m *MockPreferences) Float(key string) float64                        { return 0.0 }
func (m *MockPreferences) String(key string) string                        { return "" }
func (m *MockPreferences) BoolWithFallback(key string, f bool) bool        { return true }
func (m *MockPreferences) IntWithFallback(key string, f int) int           { return 0 }
func (m *MockPreferences) FloatWithFallback(key string, f float64) float64 { return 0.0 }
func (m *MockPreferences) StringWithFallback(key string, f string) string  { return "" }
func (m *MockPreferences) SetBool(key string, val bool)                    {}
func (m *MockPreferences) SetInt(key string, val int)                      {}
func (m *MockPreferences) SetFloat(key string, val float64)                {}

// Explicit Mock
type ExplicitMockOS struct{}

func (m *ExplicitMockOS) GetDesktopDimension() (int, int, error) {
	// Standard 16:9
	return 1920, 1080, nil
}
func (m *ExplicitMockOS) SetWallpaper(path string, monitorID int) error { return nil }
func (m *ExplicitMockOS) GetCacheDir() (string, error)                  { return "test_cache", nil }

func (m *ExplicitMockOS) GetMonitors() ([]wallpaper.Monitor, error) {
	width, height, err := m.GetDesktopDimension()
	if err != nil {
		return nil, err
	}
	return []wallpaper.Monitor{{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, width, height)}}, nil
}

type ExplicitMockPrefs struct{}

func (m *ExplicitMockPrefs) Bool(key string) bool                     { return true }
func (m *ExplicitMockPrefs) Int(key string) int                       { return 2 } // Force SmartFitAggressive
func (m *ExplicitMockPrefs) Float(key string) float64                 { return 0.0 }
func (m *ExplicitMockPrefs) String(key string) string                 { return "" }
func (m *ExplicitMockPrefs) BoolWithFallback(key string, f bool) bool { return true }
func (m *ExplicitMockPrefs) IntWithFallback(key string, f int) int {
	if key == "smart_fit_mode" {
		return 2
	}
	return f
}
func (m *ExplicitMockPrefs) FloatWithFallback(key string, f float64) float64         { return 0.0 }
func (m *ExplicitMockPrefs) StringWithFallback(key string, f string) string          { return "" }
func (m *ExplicitMockPrefs) SetBool(key string, val bool)                            {}
func (m *ExplicitMockPrefs) SetInt(key string, val int)                              {}
func (m *ExplicitMockPrefs) SetFloat(key string, val float64)                        {}
func (m *ExplicitMockPrefs) SetString(key string, val string)                        {}
func (m *ExplicitMockPrefs) AddChangeListener(callback func())                       {}
func (m *ExplicitMockPrefs) BoolList(key string) []bool                              { return nil }
func (m *ExplicitMockPrefs) BoolListWithFallback(key string, f []bool) []bool        { return nil }
func (m *ExplicitMockPrefs) SetBoolList(key string, val []bool)                      {}
func (m *ExplicitMockPrefs) IntList(key string) []int                                { return nil }
func (m *ExplicitMockPrefs) IntListWithFallback(key string, f []int) []int           { return nil }
func (m *ExplicitMockPrefs) SetIntList(key string, val []int)                        {}
func (m *ExplicitMockPrefs) FloatList(key string) []float64                          { return nil }
func (m *ExplicitMockPrefs) FloatListWithFallback(key string, f []float64) []float64 { return nil }
func (m *ExplicitMockPrefs) SetFloatList(key string, val []float64)                  {}
func (m *ExplicitMockPrefs) StringList(key string) []string                          { return nil }
func (m *ExplicitMockPrefs) StringListWithFallback(key string, f []string) []string  { return nil }
func (m *ExplicitMockPrefs) SetStringList(key string, val []string)                  {}
func (m *ExplicitMockPrefs) RemoveValue(key string)                                  {}
func (m *ExplicitMockPrefs) ChangeListeners() []func()                               { return nil }

func main() {
	prefs := &ExplicitMockPrefs{}
	cfg := wallpaper.GetConfig(prefs)
	// Force Settings
	cfg.SetSmartFitMode(wallpaper.SmartFitAggressive) // 2
	cfg.SetFaceCropEnabled(true)

	fmt.Printf("Config Mode: %d\n", cfg.GetSmartFitMode())

	mockOS := &ExplicitMockOS{}

	// Create Processor with NIL Pigo
	proc := wallpaper.NewSmartImageProcessor(mockOS, cfg, nil)

	path := "C:/Users/karlk/.gemini/antigravity/brain/e7da35b5-e9bf-4aa4-b763-20f61de16918/uploaded_image_0_1768124775172.jpg"
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Input: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())

	// Run
	res, err := proc.FitImage(context.Background(), img, 1920, 1080)
	if err != nil {
		fmt.Printf("FIT ERROR: %v\n", err)
	} else {
		fmt.Printf("FIT SUCCESS. Result: %dx%d\n", res.Bounds().Dx(), res.Bounds().Dy())
		if res.Bounds().Dx() == 1920 && res.Bounds().Dy() == 1080 {
			fmt.Println("VERIFIED RE-SIZED (Correct)")
		} else {
			fmt.Println("FAILED: DID NOT RESIZE (Incorrect)")
		}
	}
}
