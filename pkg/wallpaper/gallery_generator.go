package wallpaper

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dixieflatline76/Spice/v2/pkg/curation"
	"github.com/dixieflatline76/Spice/v2/pkg/gallery"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// GenerateGalleryForProvider processes a single curation entry and outputs the HTML cache file.
// It uses the identical logic to cmd/gallerygen/main.go.
func GenerateGalleryForProvider(ctx context.Context, prov provider.ImageProvider, entry curation.CollectionEntry, cfg *Config, client *http.Client, destDir string, limit int) error {
	thumbProv, ok := prov.(provider.ThumbnailProvider)
	if !ok {
		return fmt.Errorf("provider %s does not support thumbnails", prov.ID())
	}

	if len(entry.IDs) == 0 && len(entry.Items) == 0 {
		return nil
	}

	safeName := strings.ReplaceAll(strings.ToLower(entry.Key), " ", "_")
	fileName := fmt.Sprintf("%s.html", safeName)
	outPath := filepath.Join(destDir, fileName)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create dest dir: %w", err)
	}

	var idsToFetch []string
	if len(entry.IDs) > 0 {
		count := len(entry.IDs)
		if limit > 0 && limit < count {
			count = limit
		}
		idsToFetch = entry.IDs[:count]
	} else if len(entry.Items) > 0 {
		count := len(entry.Items)
		if limit > 0 && limit < count {
			count = limit
		}
		for _, item := range entry.Items[:count] {
			b, _ := json.Marshal(item)
			idsToFetch = append(idsToFetch, string(b))
		}
	}

	if len(idsToFetch) == 0 {
		return nil
	}

	thumbnails, err := thumbProv.FetchThumbnails(ctx, idsToFetch)
	if err != nil {
		return fmt.Errorf("failed to fetch thumbnails for %s: %w", entry.Name, err)
	}

	var wg sync.WaitGroup
	items := make([]gallery.GalleryItem, len(thumbnails))
	vFramer := NewVirtualFramer(nil, cfg)

	for i, t := range thumbnails {
		wg.Add(1)
		go func(index int, thumb provider.Thumbnail) {
			defer wg.Done()

			items[index].ID = thumb.ID
			items[index].ViewURL = thumb.ViewURL
			items[index].Title = thumb.Title
			items[index].Artist = thumb.Artist
			items[index].Year = thumb.Year

			req, err := http.NewRequestWithContext(ctx, "GET", thumb.URL, nil)
			if err != nil {
				items[index].URL = thumb.URL
				items[index].Width = 100
				items[index].Height = 100
				return
			}

			if hp, ok := prov.(provider.HeaderProvider); ok {
				for k, v := range hp.GetDownloadHeaders() {
					req.Header.Set(k, v)
				}
			}

			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == 200 {
				b, err := io.ReadAll(resp.Body)
				if err == nil {
					ct := resp.Header.Get("Content-Type")
					if ct == "" {
						ct = "image/jpeg"
					}

					img, _, err := image.Decode(bytes.NewReader(b))
					if err == nil {
						origW := img.Bounds().Dx()
						origH := img.Bounds().Dy()

						framedImg, err := vFramer.FitImage(ctx, img, origW, origH, provider.TuningOptions{
							FrameOverride: provider.FrameOverrideForceOn,
							WallColor:     provider.WallColorOverrideNeutral,
							Matting:       provider.MattingOverrideOn,
							TightCrop:     true,
						})
						if err == nil {
							items[index].Width = float64(framedImg.Bounds().Dx())
							items[index].Height = float64(framedImg.Bounds().Dy())

							var encBuf bytes.Buffer
							if err := jpeg.Encode(&encBuf, framedImg, &jpeg.Options{Quality: 90}); err == nil {
								b = encBuf.Bytes()
								ct = "image/jpeg"
							}
						} else {
							items[index].Width = float64(origW)
							items[index].Height = float64(origH)
						}
					} else {
						items[index].Width = 100
						items[index].Height = 100
					}

					b64 := base64.StdEncoding.EncodeToString(b)
					items[index].URL = fmt.Sprintf("data:%s;base64,%s", ct, b64)
				} else {
					items[index].URL = thumb.URL
					items[index].Width = 100
					items[index].Height = 100
				}
			} else {
				items[index].URL = thumb.URL
				items[index].Width = 100
				items[index].Height = 100
			}
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		}(i, t)
	}
	wg.Wait()

	var validItems []gallery.GalleryItem
	for _, item := range items {
		if item.URL != "" {
			validItems = append(validItems, item)
		}
	}

	if len(validItems) == 0 {
		return fmt.Errorf("no valid items generated for %s", entry.Name)
	}

	packedItems, wallAspect := gallery.PackSalon(validItems)

	htmlContent, err := gallery.GenerateHTML(entry.Name, entry.NameTranslations, packedItems, wallAspect, false)
	if err != nil {
		return fmt.Errorf("failed to generate HTML for %s: %w", entry.Name, err)
	}

	if err := os.WriteFile(outPath, []byte(htmlContent), 0600); err != nil {
		return fmt.Errorf("failed to write HTML for %s: %w", entry.Name, err)
	}

	log.Printf("Generator: Generated gallery for %s at %s", entry.Name, fileName)
	return nil
}
