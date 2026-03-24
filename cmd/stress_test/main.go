package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"time"
)

// Re-implementing core logic here to bypass broken Fyne dependency in the environment
const (
	SmartFitOff        = 0
	SmartFitNormal     = 1
	SmartFitAggressive = 2
)

type TuningConfig struct {
	AspectThreshold      float64
	AggressiveMultiplier float64
}

func CheckCompatibility(imgWidth, imgHeight, systemWidth, systemHeight int, mode int, tuning TuningConfig) error {
	if mode == SmartFitOff {
		return nil
	}
	if imgWidth <= 0 || imgHeight <= 0 || systemWidth <= 0 || systemHeight <= 0 {
		return nil
	}

	imageAspect := float64(imgWidth) / float64(imgHeight)
	systemAspect := float64(systemWidth) / float64(systemHeight)
	aspectDiff := math.Abs(systemAspect - imageAspect)

	if mode == SmartFitNormal {
		// Simplified Quality Check
		if aspectDiff > 0.5 {
			return fmt.Errorf("incompatible aspect ratio for Quality mode")
		}
		return nil
	}

	if mode == SmartFitAggressive {
		// FULL Flexibility Formula from smart_image_processor.go
		scaleX := float64(imgWidth) / float64(systemWidth)
		scaleY := float64(imgHeight) / float64(systemHeight)
		surplus := scaleX
		if scaleY < scaleX {
			surplus = scaleY
		}

		effectiveThreshold := tuning.AspectThreshold * surplus * tuning.AggressiveMultiplier
		if effectiveThreshold > 1.5 {
			effectiveThreshold = 1.5
		}

		// Orientation Safety
		srcLand := imgWidth > imgHeight
		tgtLand := systemWidth > systemHeight
		if srcLand != tgtLand {
			if effectiveThreshold > 0.8 {
				effectiveThreshold = 0.8
			}
		}

		if aspectDiff > effectiveThreshold {
			return fmt.Errorf("incompatible: aspect diff %.2f > threshold %.2f (surplus %.2f)", aspectDiff, effectiveThreshold, surplus)
		}
	}
	return nil
}

func main() {
	// 1. Setup
	srcDir := `C:\Users\karlk\AppData\Local\Temp\spice\wallpaper_downloads`
	
	// User settings
	targetWidth := 3440
	targetHeight := 1440
	mode := SmartFitAggressive // "Flexibility"
	tuning := TuningConfig{
		AspectThreshold:      0.9,
		AggressiveMultiplier: 2.5,
	}

	// 2. Scan and Analyze
	fmt.Printf("Scanning originals in: %s\n", srcDir)
	files, err := os.ReadDir(srcDir)
	if err != nil {
		fmt.Printf("Error reading source dir: %v\n", err)
		return
	}

	fmt.Printf("Analyzing %d files for %dx%d (Mode: Flexibility)\n", len(files), targetWidth, targetHeight)

	start := time.Now()
	passed := 0
	failed := 0
	errorCount := 0

	results := make(map[string]int)

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		ext := filepath.Ext(f.Name())
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			continue
		}

		path := filepath.Join(srcDir, f.Name())
		
		// Probing dimensions
		file, err := os.Open(path)
		if err != nil {
			errorCount++
			continue
		}
		
		config, _, err := image.DecodeConfig(file)
		file.Close()
		if err != nil {
			errorCount++
			results["decode_error"]++
			continue
		}

		// Perform Compatibility Check using our standalone function
		err = CheckCompatibility(config.Width, config.Height, targetWidth, targetHeight, mode, tuning)
		if err != nil {
			failed++
			msg := err.Error()
			// Group by prefix to avoid too many unique strings
			if len(msg) > 30 {
				msg = msg[:30] + "..."
			}
			results[msg]++
			continue
		}

		passed++
	}

	duration := time.Since(start)

	fmt.Println("\n--- STRESS TEST RESULTS ---")
	fmt.Printf("Total Files:    %d\n", len(files))
	fmt.Printf("Passed:         %d\n", passed)
	fmt.Printf("Failed:         %d\n", failed)
	fmt.Printf("Errors:         %d\n", errorCount)
	fmt.Printf("Time Taken:     %v\n", duration)

	fmt.Println("\nFailure Reasons (Sampled):")
	for reason, count := range results {
		fmt.Printf(" - %-35s: %d\n", reason, count)
	}

	if passed+failed > 0 {
		fmt.Printf("\nCompatibility Ratio: %.1f%%\n", float64(passed)/float64(passed+failed)*100)
	}
}
