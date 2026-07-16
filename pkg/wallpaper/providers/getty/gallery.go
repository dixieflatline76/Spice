package getty

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/gallery"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// GenerateGalleries generates static HTML gallery walls for all curated collections.
func (p *Provider) GenerateGalleries(ctx context.Context, destDir string) error {
	p.mu.RLock()
	col := p.collection
	p.mu.RUnlock()
	if col == nil {
		return fmt.Errorf("curated list not loaded")
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, entry := range col.Entries {
		if len(entry.IDs) == 0 {
			continue
		}

		// Take up to top 5 IDs
		count := 5
		if len(entry.IDs) < 5 {
			count = len(entry.IDs)
		}
		topIDs := entry.IDs[:count]

		var urls []string
		for _, id := range topIDs {
			// Add a small delay to avoid rate limiting during generation
			time.Sleep(200 * time.Millisecond)
			
			img, err := p.fetchObjectByUUID(ctx, id)
			if err != nil {
				log.Printf("Getty: Failed to fetch %s for gallery generation: %v", id, err)
				continue
			}
			if img != nil && img.Path != "" {
				urls = append(urls, img.Path)
			}
		}

		htmlContent, err := gallery.GenerateHTML(entry.Name, urls)
		if err != nil {
			log.Printf("Getty: Failed to generate HTML for %s: %v", entry.Name, err)
			continue
		}

		safeName := strings.ReplaceAll(strings.ToLower(entry.Key), " ", "_")
		fileName := fmt.Sprintf("%s.html", safeName)
		outPath := filepath.Join(destDir, fileName)

		if err := os.WriteFile(outPath, []byte(htmlContent), 0644); err != nil {
			log.Printf("Getty: Failed to write gallery file %s: %v", outPath, err)
			continue
		}
		log.Printf("Getty: Generated gallery %s", outPath)
	}

	return nil
}
