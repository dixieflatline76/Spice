package wallpaper

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/asset"
	pigo "github.com/esimov/pigo/core"
	"github.com/stretchr/testify/require"
)

// TestRegressionSuite runs the standard regression checks (Holistic Safety, etc.)
func TestRegressionSuite(t *testing.T) {
	// Dynamically find all images in the regression folder
	matches, err := filepath.Glob("testdata/regressions/*.jpg")
	require.NoError(t, err)
	if len(matches) == 0 {
		t.Skip("No regression images found in testdata/regressions")
	}

	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := os.Open(path)
			require.NoError(t, err)
			defer f.Close()

			img, _, err := image.Decode(f)
			require.NoError(t, err)

			ResetConfig()
			prefs := NewMockPreferences()
			cfg := GetConfig(prefs)
			cfg.SetSmartFitMode(SmartFitAggressive)
			cfg.SetFaceCropEnabled(true)

			mockOS := new(MockOS)
			mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)

			// Load Pigo
			am := asset.NewManager()
			modelData, _ := am.GetModel("facefinder")
			p := pigo.NewPigo()
			pigoInstance, _ := p.Unpack(modelData)

			processor := NewSmartImageProcessor(mockOS, cfg, pigoInstance)

			fmt.Printf("\n--- Analyzing: %s ---\n", path)
			fmt.Printf("Dimensions: %v\n", img.Bounds())

			// 1. Check Energy
			energy, _ := processor.calculateImageEnergy(context.Background(), img)
			fmt.Printf("Energy: %.4f (Threshold: %.4f)\n", energy, cfg.Tuning.MinEnergyThreshold)

			// 2. Check Faces
			faceRect, err := processor.findBestFace(img)
			if err == nil {
				fmt.Printf("Face Found: %v (Q: %.2f)\n", faceRect, processor.lastStats.Q)
			} else {
				fmt.Printf("No Face Found: %v\n", err)
			}

			// 3. Run FitImage (Decision path)
			outputImg, err := processor.FitImage(context.Background(), img)
			require.NoError(t, err)
			fmt.Printf("Result Bounds: %v\n", outputImg.Bounds())
		})
	}
}
