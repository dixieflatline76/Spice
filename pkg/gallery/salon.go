package gallery

import (
	"math"
	"sort"
)

type GalleryItem struct {
	ID      string
	URL     string
	ViewURL string
	Title   string
	Artist  string
	Year    string

	// Original pixel dimensions
	Width  float64
	Height float64

	// Calculated layout percentages (0.0 to 100.0)
	LeftPct   float64
	TopPct    float64
	WidthPct  float64
	HeightPct float64
}

// PackSalon takes a list of gallery items with Width and Height defined,
// and returns them with LeftPct, TopPct, WidthPct, and HeightPct calculated
// for a non-linear organic packing. It also returns the aspect ratio of the bounding box.
func PackSalon(items []GalleryItem) ([]GalleryItem, float64) {
	if len(items) == 0 {
		return items, 1.0
	}

	packed := make([]GalleryItem, len(items))
	copy(packed, items)

	type ptrItem struct {
		origIdx int
		item    *GalleryItem
	}
	var ptrs []ptrItem
	for i := range packed {
		ptrs = append(ptrs, ptrItem{origIdx: i, item: &packed[i]})
	}

	maxArea := 0.0
	maxIdx := 0
	for i, p := range ptrs {
		area := p.item.Width * p.item.Height
		if area > maxArea {
			maxArea = area
			maxIdx = i
		}
	}

	// Scale the centerpiece down so it doesn't dominate.
	// Real salon walls have maybe a 1.5-2x ratio between largest and smallest.
	ptrs[maxIdx].item.Width *= 0.55
	ptrs[maxIdx].item.Height *= 0.55

	for i, p := range ptrs {
		if i == maxIdx {
			continue
		}
		h := 0
		for j := 0; j < len(p.item.ID); j++ {
			h = 31*h + int(p.item.ID[j])
		}
		if h < 0 {
			h = -h
		}
		// Scale between 0.30 and 0.50 deterministically based on ID
		// This keeps secondary pieces close in size to the centerpiece (0.55)
		scale := 0.30 + float64(h%200)/1000.0
		p.item.Width *= scale
		p.item.Height *= scale
	}

	sort.Slice(ptrs, func(i, j int) bool {
		areaI := ptrs[i].item.Width * ptrs[i].item.Height
		areaJ := ptrs[j].item.Width * ptrs[j].item.Height
		return areaI > areaJ
	})

	type rect struct {
		x, y, w, h float64
	}
	var placed []rect

	overlaps := func(r rect) bool {
		for _, p := range placed {
			if r.x < p.x+p.w && r.x+r.w > p.x &&
				r.y < p.y+p.h && r.y+r.h > p.y {
				return true
			}
		}
		return false
	}

	for _, p := range ptrs {
		it := p.item
		gap := ptrs[0].item.Width * 0.04 // static gap proportional to centerpiece
		w := it.Width + gap
		h := it.Height + gap

		if len(placed) == 0 {
			placed = append(placed, rect{x: -w / 2, y: -h / 2, w: w, h: h})
			it.LeftPct = -w/2 + gap/2
			it.TopPct = -h/2 + gap/2
			continue
		}

		bestDist := math.MaxFloat64
		bestX, bestY := 0.0, 0.0
		foundAtRadius := false

		// Simple spiral search with finer steps for tighter packing
		for radius := 0.0; radius < 15000.0; radius += 15.0 {
			circumference := 2 * math.Pi * radius
			steps := int(circumference / 15.0)
			if steps < 1 {
				steps = 1
			}
			angleStep := 2 * math.Pi / float64(steps)

			for a := 0; a < steps; a++ {
				angle := float64(a) * angleStep
				cx := radius * math.Cos(angle)
				cy := radius * math.Sin(angle)

				cand := rect{x: cx - w/2, y: cy - h/2, w: w, h: h}

				if !overlaps(cand) {
					dist := math.Sqrt(cx*cx + cy*cy)
					if dist < bestDist {
						bestDist = dist
						bestX = cand.x
						bestY = cand.y
						foundAtRadius = true
					}
				}
			}
			if foundAtRadius {
				break
			}
		}

		placed = append(placed, rect{x: bestX, y: bestY, w: w, h: h})
		it.LeftPct = bestX + gap/2
		it.TopPct = bestY + gap/2
	}

	var minX, minY = math.MaxFloat64, math.MaxFloat64
	var maxX, maxY = -math.MaxFloat64, -math.MaxFloat64

	for i := range packed {
		it := &packed[i]

		if it.LeftPct < minX {
			minX = it.LeftPct
		}
		if it.TopPct < minY {
			minY = it.TopPct
		}
		if it.LeftPct+it.Width > maxX {
			maxX = it.LeftPct + it.Width
		}
		if it.TopPct+it.Height > maxY {
			maxY = it.TopPct + it.Height
		}
	}

	totalW := maxX - minX
	totalH := maxY - minY

	if totalW == 0 {
		totalW = 1
	}
	if totalH == 0 {
		totalH = 1
	}

	for i := range packed {
		it := &packed[i]
		it.LeftPct = ((it.LeftPct - minX) / totalW) * 100.0
		it.TopPct = ((it.TopPct - minY) / totalH) * 100.0
		it.WidthPct = (it.Width / totalW) * 100.0
		it.HeightPct = (it.Height / totalH) * 100.0
	}

	wallAspect := totalW / totalH
	return packed, wallAspect
}
