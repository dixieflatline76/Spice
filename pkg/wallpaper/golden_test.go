package wallpaper

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"

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

	update := os.Getenv("UPDATE_GOLDENS") == "1"
	var expected map[string]GoldenRecord
	if err := json.Unmarshal(data, &expected); err != nil {
		if !update {
			t.Fatalf("Failed to parse golden_expected.json: %v", err)
		}
		expected = make(map[string]GoldenRecord)
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

	// Read files from dir to iterate
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read golden testdata directory: %v", err)
	}

	for _, entry := range entries {
		filename := entry.Name()
		if filepath.Ext(filename) != ".jpg" && filepath.Ext(filename) != ".png" {
			continue
		}

		t.Run(filename, func(t *testing.T) {
			record, hasRecord := expected[filename]
			if !hasRecord && !update {
				t.Fatalf("No golden record for %s", filename)
			}

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

			// Compute Average Hash (aHash) for fuzzy image matching
			small := imaging.Resize(outImg, 8, 8, imaging.Linear)
			var sum uint64
			var grayVals [64]uint32
			i := 0
			bounds := small.Bounds()
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					r, g, b, _ := small.At(x, y).RGBA()
					gray := (299*(r>>8) + 587*(g>>8) + 114*(b>>8)) / 1000
					grayVals[i] = gray
					sum += uint64(gray)
					i++
				}
			}
			avg := uint32(sum / 64)
			var imgHash uint64
			for j := 0; j < 64; j++ {
				if grayVals[j] >= avg {
					imgHash |= (1 << j)
				}
			}
			hashStr := fmt.Sprintf("%016x", imgHash)
			rectStr := fmt.Sprintf("%+v", stats.Rect)

			if update {
				expected[filename] = GoldenRecord{
					Hash:      hashStr,
					FaceFound: stats.Found,
					FaceRect:  rectStr,
				}
			} else {
				var expectedHash uint64
				fmt.Sscanf(record.Hash, "%x", &expectedHash)

				// Hamming distance
				dist := 0
				val := imgHash ^ expectedHash
				for val > 0 {
					dist++
					val &= val - 1
				}

				// Allow up to 2 bits difference in 64-bit aHash
				if dist > 2 {
					t.Errorf("Visual regression detected! aHash mismatch (dist %d).\nGot:  %s\nWant: %s", dist, hashStr, record.Hash)
				}

				if stats.Found != record.FaceFound {
					t.Errorf("Face detection mismatch! Got found=%v, Want found=%v", stats.Found, record.FaceFound)
				}

				if stats.Found && record.FaceFound {
					var wMinX, wMinY, wMaxX, wMaxY int
					_, err := fmt.Sscanf(record.FaceRect, "(%d,%d)-(%d,%d)", &wMinX, &wMinY, &wMaxX, &wMaxY)
					if err != nil {
						// Fallback if the golden record was saved with %+v instead of %v
						fmt.Sscanf(record.FaceRect, "{Min:{X:%d Y:%d} Max:{X:%d Y:%d}}", &wMinX, &wMinY, &wMaxX, &wMaxY)
					}

					diff := func(a, b int) int {
						if a > b {
							return a - b
						}
						return b - a
					}

					if diff(stats.Rect.Min.X, wMinX) > 2 || diff(stats.Rect.Min.Y, wMinY) > 2 ||
						diff(stats.Rect.Max.X, wMaxX) > 2 || diff(stats.Rect.Max.Y, wMaxY) > 2 {
						t.Errorf("Face coordinates out of tolerance (±2)! Got %s, Want %s", rectStr, record.FaceRect)
					}
				}
			}
		})
	}

	if update {
		outData, err := json.MarshalIndent(expected, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal golden expected: %v", err)
		}
		if err := os.WriteFile(jsonPath, outData, 0644); err != nil {
			t.Fatalf("Failed to save golden_expected.json: %v", err)
		}
	}
}
