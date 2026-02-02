package wallpaper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dixieflatline76/Spice/util"
	"github.com/dixieflatline76/Spice/util/log"
)

// FetchNewImages iterates over active queries and submits new image jobs to the pipeline.
func (wp *Plugin) FetchNewImages() {
	if wp.fetchingInProgress.CompareAndSwap(false, true) {
		go func() {
			defer wp.fetchingInProgress.Set(false)
			log.Println("Starting image fetch from active queries...")

			wp.downloadMutex.RLock()
			queries := wp.cfg.Queries
			wp.downloadMutex.RUnlock()

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

				// Get or create per-query page counter
				wp.downloadMutex.Lock()
				pg, ok := wp.queryPages[q.ID]
				if !ok {
					pg = util.NewSafeIntWithValue(1)
					wp.queryPages[q.ID] = pg
				}
				wp.downloadMutex.Unlock()

				page := pg.Value()
				log.Printf("Fetching from provider: %s (Query: %s, Page: %d)", q.Provider, q.Description, page)
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
				queuedForThisQuery := 0

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
						queuedForThisQuery++
					} else {
						log.Printf("WARN: Pipeline full or stopped. Dropping job.")
					}
				}

				if queuedForThisQuery > 0 {
					pg.Increment()
					log.Printf("Query %s: Successfully queued %d images. Incrementing to page %d", q.ID, queuedForThisQuery, pg.Value())
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
		// Master Reset: Reset all query pages to 1
		wp.downloadMutex.Lock()
		for id := range wp.queryPages {
			wp.queryPages[id].Set(1)
		}
		wp.downloadMutex.Unlock()

		// Robust Sync: Reconcile store and invalidate stale derivatives
		wp.syncStoreWithConfig()

		// Trigger immediate fetch
		wp.FetchNewImages()

		// Wait for images using event driven notification (up to 15s)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		log.Println("[Init] Waiting for images before initial pulse...")
		if err := wp.store.WaitForImages(ctx); err == nil {
			log.Println("[Init] Images available. Triggering initial pulse.")
			// Use dispatch directly to bypass Stagger logic (Force Immediate)
			wp.dispatch(-1, CmdNext)
		} else {
			log.Println("[Init] Initial pulse timeout. Triggering anyway.")
			wp.dispatch(-1, CmdNext)
		}
	}()
}
