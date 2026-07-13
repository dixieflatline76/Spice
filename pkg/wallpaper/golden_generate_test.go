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

type GoldenRecord struct {
	Hash      string `json:"hash"`
	FaceFound bool   `json:"face_found"`
	FaceRect  string `json:"face_rect"`
}

// TestGenerateGolden is a utility test to generate the golden_expected.json file.
// Run this with: go test -v -run TestGenerateGolden ./pkg/wallpaper/...
func TestGenerateGolden(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") != "1" {
		t.Skip("Skipping golden file generation. Run with UPDATE_GOLDEN=1 to update baselines.")
	}

	dir := filepath.Join("testdata", "golden")
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("No golden directory found: %v", err)
	}

	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitAggressive)
	cfg.SetFaceCropEnabled(true)
	cfg.VirtualFramingFallback = true
	cfg.VirtualPaperMatting = false

	mockOS := new(MockOS)
	mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)

	// Load Pigo
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

	results := make(map[string]GoldenRecord)
	for _, f := range files {
		if f.IsDir() || (filepath.Ext(f.Name()) != ".jpg" && filepath.Ext(f.Name()) != ".jpeg" && filepath.Ext(f.Name()) != ".png") {
			continue
		}

		smartProcessor := NewSmartImageProcessor(mockOS, cfg, pigoInstance)
		processor := NewVirtualFramer(smartProcessor, cfg)
		ctx := context.Background()

		path := filepath.Join(dir, f.Name())
		fmt.Printf("Processing %s...\n", f.Name())

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
			t.Fatalf("Failed to fit %s: %v", f.Name(), err)
		}

		stats := smartProcessor.GetLastStats()

		hasher := sha256.New()
		bounds := outImg.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := outImg.At(x, y).RGBA()
				hasher.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)})
			}
		}
		hash := hex.EncodeToString(hasher.Sum(nil))

		results[f.Name()] = GoldenRecord{
			Hash:      hash,
			FaceFound: stats.Found,
			FaceRect:  fmt.Sprintf("%+v", stats.Rect),
		}
	}

	outPath := filepath.Join(dir, "golden_expected.json")
	data, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		t.Fatalf("Failed to write golden file: %v", err)
	}
	fmt.Printf("Wrote golden expected results to %s\n", outPath)
}
