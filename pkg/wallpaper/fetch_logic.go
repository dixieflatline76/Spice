package wallpaper

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util"
	"github.com/dixieflatline76/Spice/util/log"
)

// FetchNewImages iterates over active queries and submits new image jobs to the pipeline.
// If providerID is specified, only queries for that provider are fetched.
func (wp *Plugin) FetchNewImages(providerID ...string) {
	targetProvider := ""
	if len(providerID) > 0 {
		targetProvider = providerID[0]
	}

	// Special-case Favorites for on-the-fly responsiveness
	isFavRequest := targetProvider == "Favorites"

	if isFavRequest || wp.fetchingInProgress.CompareAndSwap(false, true) {
		go func() {
			if !isFavRequest {
				defer wp.fetchingInProgress.Set(false)
			}
			log.Printf("Starting image fetch (Target: %s)...", func() string {
				if targetProvider == "" {
					return "ALL"
				}
				return targetProvider
			}())

			wp.downloadMutex.RLock()
			if wp.cfg == nil {
				wp.downloadMutex.RUnlock()
				return
			}
			wp.cfg.mu.Lock()
			queries := make([]ImageQuery, len(wp.cfg.Queries))
			copy(queries, wp.cfg.Queries)
			wp.cfg.mu.Unlock()
			wp.downloadMutex.RUnlock()

			totalQueued := util.NewSafeInt()
			activeSources := make(map[string]bool)
			var sourcesMutex sync.Mutex

			// Semaphore to limit concurrent fetches
			sem := make(chan struct{}, 5)
			var wg sync.WaitGroup

			for _, q := range queries {
				if !q.Active {
					continue
				}

				// Targeted Fetch filter
				if targetProvider != "" && q.Provider != targetProvider {
					continue
				}

				p, ok := wp.providers[q.Provider]
				if !ok {
					log.Printf("Provider %s not found for query %s", q.Provider, q.ID)
					continue
				}

				wg.Add(1)
				go func(q ImageQuery, p provider.ImageProvider) {
					defer wg.Done()
					sem <- struct{}{}        // Acquire
					defer func() { <-sem }() // Release

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

					// Add timeout to prevent hangs
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					images, err := p.FetchImages(ctx, q.URL, page)
					if err != nil {
						log.Printf("Provider %s fetch failed: %v", q.Provider, err)
						return
					}
					if len(images) == 0 {
						log.Printf("Provider %s returned no new images.", q.Provider)
						return
					}

					log.Printf("[Fetch] Provider %s returned %d images. Submitting to pipeline.", q.Provider, len(images))

					// Track source
					sourcesMutex.Lock()
					activeSources[p.Name()] = true
					sourcesMutex.Unlock()

					queuedForThisQuery := 0

					for _, img := range images {
						// Critical Fix: Tag image with its source query ID so Sync knows it's active.
						img.SourceQueryID = q.ID

						// *** NAMESPACING Middleware ***
						// Ensure ID is unique across providers by prefixing it.
						if p.Type() == provider.TypeOnline {
							prefix := p.Name() + "_"
							if !strings.HasPrefix(img.ID, prefix) {
								img.ID = prefix + img.ID
							}
						}

						if !isFavRequest && wp.cfg.InAvoidSet(img.ID) {
							log.Debugf("Skipping blocked image: %s", img.ID)
							continue
						}
						job := DownloadJob{
							Image:    img,
							Provider: p,
						}
						// Submit non-blocking
						if wp.jobSubmitter.Submit(job) {
							totalQueued.Increment()
							queuedForThisQuery++
						} else {
							log.Printf("WARN: Pipeline full or stopped. Dropping job.")
						}
					}

					if queuedForThisQuery > 0 {
						pg.Increment()
						log.Printf("Query %s: Successfully queued %d images. Incrementing to page %d", q.ID, queuedForThisQuery, pg.Value())
					}
				}(q, p)
			}

			wg.Wait()

			// Batch Reshuffle Optimization (User Approach):
			// Signal monitors to update their shuffle lists only after the entire batch is processed.
			if totalQueued.Value() > 0 {
				log.Printf("[Fetch] Processed %d new images. Broadcasting shuffle update to monitors...", totalQueued.Value())
				wp.dispatch(-1, CmdUpdateShuffle)

				sources := []string{}
				for s := range activeSources {
					sources = append(sources, s)
				}
				sourceStr := ""
				if len(sources) > 0 {
					sourceStr = " from " + strings.Join(sources, ", ")
				}
				wp.manager.NotifyUser("Wallpaper Fetch", fmt.Sprintf("Downloading %d new images%s...", totalQueued.Value(), sourceStr))
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
		// Optimization: Removed Master Reset (Page 1) to prevent cache churn on Settings Apply.
		// Pages are now persistent per session or untill Nightly Refresh.

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
