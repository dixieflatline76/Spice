package wallpaper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dixieflatline76/Spice/util/log"
)

// FetchNewImages iterates over active queries and submits new image jobs to the pipeline.
func (wp *Plugin) FetchNewImages() {
	if wp.fetchingInProgress.CompareAndSwap(false, true) {
		go func() {
			defer wp.fetchingInProgress.Set(false)
			log.Println("Starting image fetch from active queries...")

			// Iterate over configured queries in a thread-safe way?
			// cfg.Queries is slice. We should probably lock or copy?
			// But iterating slice value (copy) is safe if accessible.
			// wp.cfg is *Config. Config structs usually exported but fields might need mutex?
			// Config.Queries is exported. We should use RLocker if intended, but direct access is common if careful.
			// Actually Config has a mutex `mu`. But queries slice is exposed.
			// Ideally we use a getter or simple loop.
			// For now, simple loop access.

			queries := wp.cfg.Queries
			page := wp.currentDownloadPage.Value()
			totalQueued := 0
			activeSources := make(map[string]bool)

			for _, q := range queries {
				if !q.Active {
					continue
				}

				p, ok := wp.providers[q.Provider]
				if !ok {
					log.Printf("Provider %s not found for query %s", q.Provider, q.ID)
					continue
				}

				log.Printf("Fetching from provider: %s (Query: %s)", q.Provider, q.Description)
				// Fetch API URL. q.URL is API URL usually.
				images, err := p.FetchImages(context.Background(), q.URL, page)
				if err != nil {
					log.Printf("Provider %s fetch failed: %v", q.Provider, err)
					continue
				}
				if len(images) == 0 {
					log.Printf("Provider %s returned no new images.", q.Provider)
					continue
				}

				log.Printf("Provider %s returned %d images. Submitting to pipeline.", q.Provider, len(images))

				// Track source
				activeSources[p.Name()] = true

				for _, img := range images {
					if wp.cfg.InAvoidSet(img.ID) {
						log.Debugf("Skipping blocked image: %s", img.ID)
						continue
					}
					job := DownloadJob{
						Image:    img,
						Provider: p,
					}
					// Submit non-blocking
					if wp.pipeline.Submit(job) {
						totalQueued++
					} else {
						log.Printf("WARN: Pipeline full or stopped. Dropping job.")
					}
				}
			}

			if totalQueued > 0 {
				sources := []string{}
				for s := range activeSources {
					sources = append(sources, s)
				}
				sourceStr := ""
				if len(sources) > 0 {
					sourceStr = " from " + strings.Join(sources, ", ")
				}
				wp.manager.NotifyUser("Wallpaper Fetch", fmt.Sprintf("Downloading %d new images%s...", totalQueued, sourceStr))

				// Increment Page for next time
				wp.currentDownloadPage.Increment()
				log.Printf("Fetch successful. Incrementing page to %d", wp.currentDownloadPage.Value())

			} else {
				log.Println("Fetch returned 0 new images from all active queries.")
			}

			log.Println("Fetch cycle completed.")
		}()
	} else {
		log.Println("Fetch skipped - already in progress.")
	}
}

// RefreshImagesAndPulse triggers a fetch and then updates the wallpaper.
func (wp *Plugin) RefreshImagesAndPulse() {
	go func() {
		wp.FetchNewImages()

		// Wait for images using event driven notification (up to 15s)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		log.Println("[Init] Waiting for images before initial pulse...")
		if err := wp.store.WaitForImages(ctx); err == nil {
			log.Println("[Init] Images available. Triggering initial pulse.")
			wp.SetNextWallpaper(-1)
		} else {
			log.Println("[Init] Initial pulse timeout. Triggering anyway.")
			wp.SetNextWallpaper(-1)
		}
	}()
}
