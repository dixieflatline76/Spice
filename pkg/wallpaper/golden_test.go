package wallpaper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	pigo "github.com/esimov/pigo/core"
)

// TestGoldenMaster verifies the output of the SmartImageProcessor pipeline
// against the pre-computed snapshots in golden_expected.json.
func TestGoldenMaster(t *testing.T) {

	dir := filepath.Join("testdata", "golden")

	jsonPath := filepath.Join(dir, "golden_expected.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Golden expected file missing, skipping test: %v", err)
	}

	var expected map[string]GoldenRecord
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatalf("Failed to parse golden_expected.json: %v", err)
	}

	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitAggressive)
	cfg.SetFaceCropEnabled(true)
	cfg.VirtualFramingFallback = true
	cfg.VirtualPaperMatting = false

	mockOS := new(MockOS)
	mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)

	am := asset.NewManager()
	modelData, err := am.GetModel("facefinder")
	if err != nil {
		t.Fatalf("Failed to load facefinder: %v", err)
	}
	p := pigo.NewPigo()
	pigoInstance, err := p.Unpack(modelData)
	if err != nil {
		t.Fatalf("Failed to unpack facefinder: %v", err)
	}

	for filename, record := range expected {
		t.Run(filename, func(t *testing.T) {
			smartProcessor := NewSmartImageProcessor(mockOS, cfg, pigoInstance)
			processor := NewVirtualFramer(smartProcessor, cfg)
			ctx := context.Background()
			path := filepath.Join(dir, filename)
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("Failed to open %s: %v", path, err)
			}

			img, _, err := image.Decode(file)
			file.Close()
			if err != nil {
				t.Fatalf("Failed to decode %s: %v", path, err)
			}

			opts := provider.TuningOptions{
				Anchor: provider.AnchorAuto,
			}

			outImg, err := processor.FitImage(ctx, img, 1920, 1080, opts)
			if err != nil {
				t.Fatalf("Failed to fit %s: %v", filename, err)
			}

			stats := smartProcessor.GetLastStats()

			// Hash the output pixels
			hasher := sha256.New()
			bounds := outImg.Bounds()
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					r, g, b, a := outImg.At(x, y).RGBA()
					hasher.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)})
				}
			}
			hash := hex.EncodeToString(hasher.Sum(nil))

			if hash != record.Hash {
				t.Errorf("Visual regression detected! Hash mismatch.\nGot:  %s\nWant: %s", hash, record.Hash)
			}

			if stats.Found != record.FaceFound {
				t.Errorf("Face detection mismatch! Got found=%v, Want found=%v", stats.Found, record.FaceFound)
			}

			rectStr := fmt.Sprintf("%+v", stats.Rect)
			if rectStr != record.FaceRect {
				t.Errorf("Face coordinates mismatch! Got %s, Want %s", rectStr, record.FaceRect)
			}
		})
	}
}
