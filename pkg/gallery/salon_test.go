package gallery

import (
	"math"
	"testing"
)

func TestPackSalon_SingleImage(t *testing.T) {
	items := []GalleryItem{
		{Width: 800, Height: 600},
	}

	packed, _ := PackSalon(items)
	if len(packed) != 1 {
		t.Fatalf("Expected 1 packed item, got %d", len(packed))
	}

	// For a single image, it should take up 100% of the calculated space
	if !approxEqual(packed[0].LeftPct, 0) || !approxEqual(packed[0].TopPct, 0) {
		t.Errorf("Single image should be at (0,0), got (%.2f, %.2f)", packed[0].LeftPct, packed[0].TopPct)
	}
	if !approxEqual(packed[0].WidthPct, 100) || !approxEqual(packed[0].HeightPct, 100) {
		t.Errorf("Single image should take 100%% of width and height, got w:%.2f, h:%.2f", packed[0].WidthPct, packed[0].HeightPct)
	}
}

func TestPackSalon_NoOverlaps(t *testing.T) {
	items := []GalleryItem{
		{ID: "1", Width: 800, Height: 600},
		{ID: "2", Width: 400, Height: 800},
		{ID: "3", Width: 600, Height: 600},
		{ID: "4", Width: 1200, Height: 400},
		{ID: "5", Width: 500, Height: 500},
	}

	packed, _ := PackSalon(items)

	// Check that no two rectangles overlap
	for i := 0; i < len(packed); i++ {
		for j := i + 1; j < len(packed); j++ {
			r1 := packed[i]
			r2 := packed[j]

			// Rectangles in % space
			r1Right := r1.LeftPct + r1.WidthPct
			r1Bottom := r1.TopPct + r1.HeightPct

			r2Right := r2.LeftPct + r2.WidthPct
			r2Bottom := r2.TopPct + r2.HeightPct

			// Check intersection
			if r1.LeftPct < r2Right && r1Right > r2.LeftPct &&
				r1.TopPct < r2Bottom && r1Bottom > r2.TopPct {
				t.Errorf("Overlap detected between item %s and item %s", r1.ID, r2.ID)
			}
		}
	}
}

func TestPackSalon_NormalizationBounds(t *testing.T) {
	items := []GalleryItem{
		{ID: "1", Width: 800, Height: 600},
		{ID: "2", Width: 400, Height: 800},
		{ID: "3", Width: 600, Height: 600},
	}

	packed, _ := PackSalon(items)

	var minX, minY float64 = math.MaxFloat64, math.MaxFloat64
	var maxX, maxY float64 = -math.MaxFloat64, -math.MaxFloat64

	for _, p := range packed {
		if p.LeftPct < minX {
			minX = p.LeftPct
		}
		if p.TopPct < minY {
			minY = p.TopPct
		}
		if p.LeftPct+p.WidthPct > maxX {
			maxX = p.LeftPct + p.WidthPct
		}
		if p.TopPct+p.HeightPct > maxY {
			maxY = p.TopPct + p.HeightPct
		}
	}

	if !approxEqual(minX, 0) || !approxEqual(minY, 0) {
		t.Errorf("Bounding box min should be (0,0), got (%.2f, %.2f)", minX, minY)
	}
	if !approxEqual(maxX, 100) || !approxEqual(maxY, 100) {
		t.Errorf("Bounding box max should be (100,100), got (%.2f, %.2f)", maxX, maxY)
	}
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}
