package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
)

func main() {
	path := "C:/Users/karlk/.gemini/antigravity/brain/e7da35b5-e9bf-4aa4-b763-20f61de16918/uploaded_image_0_1768124775172.jpg"
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		fmt.Printf("Error decoding config: %v\n", err)
		return
	}

	imgW, imgH := cfg.Width, cfg.Height
	sysW, sysH := 1920, 1080 // Standard 16:9

	imageAspect := float64(imgW) / float64(imgH)
	systemAspect := float64(sysW) / float64(sysH)
	aspectDiff := math.Abs(systemAspect - imageAspect)

	safeThreshold := 0.5

	fmt.Printf("File: %s\n", path)
	fmt.Printf("Dimensions: %dx%d\n", imgW, imgH)
	fmt.Printf("Image Aspect: %.6f\n", imageAspect)
	fmt.Printf("System Aspect: %.6f (16:9)\n", systemAspect)
	fmt.Printf("Diff: %.6f\n", aspectDiff)
	fmt.Printf("Threshold: %.6f\n", safeThreshold)

	if aspectDiff > safeThreshold {
		fmt.Println("Result: UNSAFE -> CENTER CROP (Correct behavior)")
	} else {
		fmt.Println("Result: SAFE -> SMART CROP (Problem: Feet Crop)")
	}
}
