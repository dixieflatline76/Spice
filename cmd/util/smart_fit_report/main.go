package main

import (
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
	pigo "github.com/esimov/pigo/core"
)

type TestImage struct {
	Name string
	URL  string
}

// DummyOS implements wallpaper.OS
type DummyOS struct{}

func (d *DummyOS) GetDesktopDimension() (int, int, error) {
	return 3440, 1440, nil
}
func (d *DummyOS) SetWallpaper(path string) error {
	return nil
}

// MockPreferences satisfies fyne.Preferences
type MockPreferences struct {
	data map[string]interface{}
}

func NewMockPreferences() *MockPreferences {
	return &MockPreferences{data: make(map[string]interface{})}
}

func (m *MockPreferences) Bool(key string) bool {
	if v, ok := m.data[key]; ok {
		return v.(bool)
	}
	return false
}
func (m *MockPreferences) BoolWithFallback(key string, fallback bool) bool {
	if v, ok := m.data[key]; ok {
		return v.(bool)
	}
	return fallback
}
func (m *MockPreferences) SetBool(key string, value bool) { m.data[key] = value }
func (m *MockPreferences) Int(key string) int {
	if v, ok := m.data[key]; ok {
		return v.(int)
	}
	return 0
}
func (m *MockPreferences) IntWithFallback(key string, fallback int) int {
	if v, ok := m.data[key]; ok {
		return v.(int)
	}
	return fallback
}
func (m *MockPreferences) SetInt(key string, value int) { m.data[key] = value }
func (m *MockPreferences) String(key string) string {
	if v, ok := m.data[key]; ok {
		return v.(string)
	}
	return ""
}
func (m *MockPreferences) StringWithFallback(key string, fallback string) string {
	if v, ok := m.data[key]; ok {
		return v.(string)
	}
	return fallback
}
func (m *MockPreferences) SetString(key string, value string)                     { m.data[key] = value }
func (m *MockPreferences) Float(key string) float64                               { return 0.0 }
func (m *MockPreferences) FloatWithFallback(key string, fallback float64) float64 { return fallback }
func (m *MockPreferences) SetFloat(key string, value float64)                     {}
func (m *MockPreferences) RemoveValue(key string)                                 {}
func (m *MockPreferences) AddChangeListener(func())                               {}
func (m *MockPreferences) ChangeListeners() []func()                              { return nil }

// Implement missing list methods to satisfy fyne.Preferences
func (m *MockPreferences) BoolList(key string) []bool                              { return nil }
func (m *MockPreferences) BoolListWithFallback(key string, fallback []bool) []bool { return fallback }
func (m *MockPreferences) SetBoolList(key string, value []bool)                    {}
func (m *MockPreferences) IntList(key string) []int                                { return nil }
func (m *MockPreferences) IntListWithFallback(key string, fallback []int) []int    { return fallback }
func (m *MockPreferences) SetIntList(key string, value []int)                      {}
func (m *MockPreferences) FloatList(key string) []float64                          { return nil }
func (m *MockPreferences) FloatListWithFallback(key string, fallback []float64) []float64 {
	return fallback
}
func (m *MockPreferences) SetFloatList(key string, value []float64) {}
func (m *MockPreferences) StringList(key string) []string           { return nil }
func (m *MockPreferences) StringListWithFallback(key string, fallback []string) []string {
	return fallback
}
func (m *MockPreferences) SetStringList(key string, value []string) {}

func main() {
	log.Println("Starting Smart Fit Report Generator...")

	outputDir := "report_output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output dir: %v", err)
	}

	// 1. Initialize Pigo
	log.Println("Loading Face Detection Model...")
	var pigoInstance *pigo.Pigo
	am := asset.NewManager()
	modelData, err := am.GetModel("facefinder")
	if err != nil {
		log.Printf("Warning: Failed to load face detection model: %v. Face Boost will be disabled.", err)
	} else {
		p := pigo.NewPigo()
		pigoInstance, err = p.Unpack(modelData)
		if err != nil {
			log.Printf("Warning: Failed to unpack model: %v", err)
		} else {
			log.Println("Face Detection Model Loaded.")
		}
	}

	// 2. Setup Dependencies
	mockPrefs := NewMockPreferences()
	cfg := &wallpaper.Config{
		Preferences: mockPrefs,
	}

	// Dummy OS
	dummyAddr := &DummyOS{}

	// Initialize Processor
	processor := wallpaper.NewSmartImageProcessor(dummyAddr, cfg, pigoInstance)

	// HTML Report Buffer
	var html strings.Builder
	html.WriteString(`<html><head><style>
		body { font-family: sans-serif; background: #222; color: #eee; padding: 20px; }
		.test-case { margin-bottom: 50px; border-bottom: 1px solid #444; padding-bottom: 20px; }
		h2 { color: #f0a500; }
		.grid { display: grid; grid-template-columns: repeat(5, 1fr); gap: 10px; }
		.cell { text-align: center; }
		img { max-width: 100%; height: auto; border: 2px solid #555; }
		.label { margin-top: 5px; font-size: 0.9em; color: #aaa; }
		.meta { font-size: 0.8em; color: #777; }
	</style></head><body><h1>Smart Fit 2.0 Report (3440x1440 Ultrawide Test)</h1>`)

	ctx := context.Background()

	// 3. Scan test_assets/tuning_images for test images
	sourceDir := filepath.Join("test_assets", "tuning_images")
	tmpFiles, err := os.ReadDir(sourceDir)
	if err != nil {
		log.Fatalf("Failed to read source directory %s: %v", sourceDir, err)
	}

	var testImages []TestImage
	for _, f := range tmpFiles {
		if !f.IsDir() && (strings.HasSuffix(f.Name(), ".jpg") || strings.HasSuffix(f.Name(), ".png")) {
			// Construct absolute path for imaging.Open
			absPath, _ := filepath.Abs(filepath.Join(sourceDir, f.Name()))
			testImages = append(testImages, TestImage{
				Name: strings.TrimSuffix(f.Name(), filepath.Ext(f.Name())),
				URL:  "file:///" + filepath.ToSlash(absPath),
			})
		}
	}

	if len(testImages) == 0 {
		log.Fatal("No images found in test_assets/tuning_images")
	}

	// 4. Loop Images
	for _, ti := range testImages {
		log.Printf("Processing %s...", ti.Name)
		html.WriteString(fmt.Sprintf(`<div class="test-case"><h2>%s</h2><div class="grid">`, ti.Name))

		// Get Image
		var srcImg image.Image
		var origDisplayPath string

		localPath := strings.TrimPrefix(ti.URL, "file:///")
		srcImg, err = imaging.Open(localPath)
		if err != nil {
			log.Printf("Failed to open local %s: %v", localPath, err)
			continue
		}
		// Copy to output for HTML display
		destPath := filepath.Join(outputDir, ti.Name+"_original"+filepath.Ext(localPath))
		if err := copyFile(localPath, destPath); err != nil {
			log.Printf("Failed to copy %s: %v", localPath, err)
		}
		origDisplayPath = filepath.Base(destPath)

		// Add Original to Grid
		html.WriteString(fmt.Sprintf(`
			<div class="cell">
				<img src="%s" />
				<div class="label">Original</div>
				<div class="meta">%dx%d</div>
			</div>`, origDisplayPath, srcImg.Bounds().Dx(), srcImg.Bounds().Dy()))

		// Define Modes
		// Define Modes
		modes := []struct {
			Name      string
			SmartFit  bool
			Mode      wallpaper.SmartFitMode
			FaceCrop  bool
			FaceBoost bool
			BoostStr  int
		}{
			{"Standard (Quality)", true, wallpaper.SmartFitNormal, false, false, 0},
			{"Standard (Flexibility)", true, wallpaper.SmartFitAggressive, false, false, 0},
			{"Face Crop (Flex)", true, wallpaper.SmartFitAggressive, true, false, 0},
			{"Face Boost (S0) (Flex)", true, wallpaper.SmartFitAggressive, false, true, 0},
		}

		for _, m := range modes {
			// Configure Mock Preferences
			mockPrefs.SetBool(wallpaper.SmartFitPrefKey, m.SmartFit)
			mockPrefs.SetInt(wallpaper.SmartFitModePrefKey, int(m.Mode))
			mockPrefs.SetBool(wallpaper.FaceCropPrefKey, m.FaceCrop)
			mockPrefs.SetBool(wallpaper.FaceBoostPrefKey, m.FaceBoost)
			mockPrefs.SetInt(wallpaper.FaceBoostStrengthPrefKey, m.BoostStr)

			// Note: Using application defaults (1% MinSize, 5.0 Conf, 0.1 Shift)
			// Explicitly remove any overrides to ensure we test defaults
			mockPrefs.RemoveValue(wallpaper.FaceDetectMinSizePctPrefKey)
			mockPrefs.RemoveValue(wallpaper.FaceDetectConfPrefKey)
			mockPrefs.RemoveValue(wallpaper.FaceDetectShiftPrefKey)

			// Process
			resImg, err := processor.FitImage(ctx, srcImg)
			stats := processor.GetLastStats()

			filename := fmt.Sprintf("%s_%s.jpg", ti.Name, sanitize(m.Name))
			outPath := filepath.Join(outputDir, filename)

			if err != nil {
				log.Printf("Error processing %s [%s]: %v", ti.Name, m.Name, err)
				html.WriteString(fmt.Sprintf(`<div class="cell" style="color:#ff6b6b">Error: %v</div>`, err))
			} else {
				// Save Result
				if err := imaging.Save(resImg, outPath); err != nil {
					log.Printf("Error saving %s: %v", outPath, err)
				}

				faceInfo := "No Face detected"
				if stats.Found {
					faceInfo = fmt.Sprintf("Face: (Q:%.1f, S:%d)<br>Rect: %v", stats.Q, stats.Scale, stats.Rect)
				}

				html.WriteString(fmt.Sprintf(`
					<div class="cell">
						<img src="%s" />
						<div class="label">%s</div>
						<div class="meta">%dx%d<br>%v<br>%s</div>
					</div>`, filename, m.Name, resImg.Bounds().Dx(), resImg.Bounds().Dy(), stats.Processing.Round(time.Millisecond), faceInfo))
			}
		}

		html.WriteString(`</div></div>`)
	}

	html.WriteString(`</body></html>`)

	// Save Report
	if err := os.WriteFile(filepath.Join(outputDir, "report.html"), []byte(html.String()), 0644); err != nil {
		log.Fatalf("Failed to save report: %v", err)
	}

	log.Println("Report generated successfully at report_output/report.html")
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "(", "")
	s = strings.ReplaceAll(s, ")", "")
	return s
}
