package npm

import (
	"context"
	"net/http"
	"testing"
)

func TestNPMProvider_fetchImageByCID(t *testing.T) {
	p := NewProvider(nil, http.DefaultClient)

	// CID 2383 is the Octagonal inkstick
	img, err := p.fetchImageByCID(context.Background(), 2383)
	if err != nil {
		t.Fatalf("fetchImageByCID failed: %v", err)
	}

	if img == nil {
		t.Fatal("expected image, got nil")
	}

	expectedURL := "https://iiifod.npm.gov.tw/iiif/2/K1F%2FK1F000001N000000000PAB/full/max/0/default.jpg"

	if img.Path != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, img.Path)
	}

	if img.Attribution == "" {
		t.Error("expected non-empty attribution")
	}

	if img.ViewURL != "https://digitalarchive.npm.gov.tw/Collection/Detail?id=2383&dep=U" {
		t.Errorf("unexpected view URL: %s", img.ViewURL)
	}
}

func TestSelectBestCanvasID(t *testing.T) {
	// Helper function to build a mock manifest
	buildManifest := func(canvases ...struct{ w, h, id string }) *iiifManifest {
		m := &iiifManifest{}
		m.Sequences = make([]struct {
			Canvases []struct {
				Width  string `json:"width"`
				Height string `json:"height"`
				Images []struct {
					Resource struct {
						Service struct {
							ID string `json:"@id"`
						} `json:"service"`
					} `json:"resource"`
				} `json:"images"`
			} `json:"canvases"`
		}, 1)

		for _, c := range canvases {
			canvas := struct {
				Width  string `json:"width"`
				Height string `json:"height"`
				Images []struct {
					Resource struct {
						Service struct {
							ID string `json:"@id"`
						} `json:"service"`
					} `json:"resource"`
				} `json:"images"`
			}{
				Width:  c.w,
				Height: c.h,
			}
			canvas.Images = make([]struct {
				Resource struct {
					Service struct {
						ID string `json:"@id"`
					} `json:"service"`
				} `json:"resource"`
			}, 1)
			canvas.Images[0].Resource.Service.ID = c.id
			m.Sequences[0].Canvases = append(m.Sequences[0].Canvases, canvas)
		}
		return m
	}

	tests := []struct {
		name     string
		manifest *iiifManifest
		expected string
	}{
		{
			name: "Single canvas",
			manifest: buildManifest(
				struct{ w, h, id string }{"1000", "1000", "ID_0"},
			),
			expected: "ID_0",
		},
		{
			name: "Two canvases, first is tag",
			manifest: buildManifest(
				struct{ w, h, id string }{"500", "500", "TAG_ID"},
				struct{ w, h, id string }{"1500", "1500", "HERO_ID"},
			),
			expected: "HERO_ID",
		},
		{
			name: "Index 1 is low-res, Index 2 is hero",
			manifest: buildManifest(
				struct{ w, h, id string }{"500", "500", "TAG_ID"},
				struct{ w, h, id string }{"200", "200", "LOW_RES"},
				struct{ w, h, id string }{"1000", "1000", "HERO_ID"},
			),
			expected: "HERO_ID",
		},
		{
			name: "Index 1 is hero, Index 3 is macro seal (higher res)",
			manifest: buildManifest(
				struct{ w, h, id string }{"500", "500", "TAG_ID"},
				struct{ w, h, id string }{"1000", "1000", "HERO_ID"},
				struct{ w, h, id string }{"1000", "1000", "ALT_HERO"},
				struct{ w, h, id string }{"2000", "2000", "MACRO_BASE"}, // Much higher res, but we should stop at HERO_ID
			),
			expected: "HERO_ID",
		},
		{
			name: "All canvases under 800k, pick the highest scanned (from idx 1)",
			manifest: buildManifest(
				struct{ w, h, id string }{"500", "500", "TAG_ID"},
				struct{ w, h, id string }{"200", "200", "TINY"},
				struct{ w, h, id string }{"700", "700", "BEST_SUB_800K"},
				struct{ w, h, id string }{"300", "300", "SMALL"},
			),
			expected: "BEST_SUB_800K",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectBestCanvasID(tt.manifest)
			if got != tt.expected {
				t.Errorf("selectBestCanvasID() = %v, want %v", got, tt.expected)
			}
		})
	}
}
