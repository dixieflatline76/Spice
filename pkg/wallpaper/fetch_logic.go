package wallpaper

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// FetchNewImages iterates over active queries and submits new image jobs to the pipeline.
// If force is true, it proceeds even if another fetch is in progress (ignoring the debounce lock).
// If providerID is specified, only queries for that provider are fetched.
func (wp *Plugin) FetchNewImages(force bool, providerID ...string) {
	targetProvider := ""
	if len(providerID) > 0 {
		targetProvider = providerID[0]
	}

	// Special-case Favorites for on-the-fly responsiveness
	isFavRequest := targetProvider == "Favorites"

	if force || isFavRequest || wp.fetchingInProgress.CompareAndSwap(false, true) {
		go func() {
			if !isFavRequest && !force {
				defer wp.fetchingInProgress.Set(false)
			}
			log.Debugf("Starting image fetch (Target: %s)...", func() string {
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

			// Initialize the global fetch context to allow remote aborts
			fetchCtx := wp.StartFetchContext()

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

					// Pattern: Early Exit (Circuit Breaker)
					if tp, ok := p.(provider.ThrottledProvider); ok {
						if tp.IsThrottled() {
							log.Printf("Provider %s is currently throttled. Skipping fetch for query %s.", p.ID(), q.ID)
							return
						}
					}

					// Pattern: Pacing Penalty OUTSIDE of CPU semaphore
					// Wait freely without holding any execution lock so we don't starve fast providers!
					if limiter := wp.getAPILimiter(p); limiter != nil {
						log.Debugf("[Pacing] Waiting for API rate limiter slot for provider %s...", p.ID())
						if err := limiter.Wait(fetchCtx); err != nil {
							log.Printf("Provider %s fetch aborted due to context cancellation during pacing: %v", p.ID(), err)
							return
						}
					}

					log.Debugf("[Fetch] %s Waiting for slot... (Lane Load: %d/%d)", p.ID(), len(sem), cap(sem))
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-fetchCtx.Done():
						return
					}

					wp.fetchFromProvider(fetchCtx, q, p, isFavRequest, &sourcesMutex, activeSources, totalQueued)
				}(q, p)
			}

			wg.Wait()

			// Batch Reshuffle Optimization (User Approach):
			// Signal monitors to update their shuffle lists only after the entire batch is processed.
			if totalQueued.Value() > 0 {
				log.Debugf("[Fetch] Processed %d new images. Broadcasting shuffle update to monitors...", totalQueued.Value())
				wp.dispatch(-1, CmdUpdateShuffle)

				sources := []string{}
				for s := range activeSources {
					sources = append(sources, s)
				}
				sourceStr := strings.Join(sources, ", ")
				if sourceStr != "" {
					wp.manager.NotifyUser(i18n.T("Wallpaper Fetch"), i18n.Tf("Downloading {{.Count}} new images from {{.Sources}}...", map[string]any{"Count": totalQueued.Value(), "Sources": sourceStr}))
				} else {
					wp.manager.NotifyUser(i18n.T("Wallpaper Fetch"), i18n.Tf("Downloading {{.Count}} new images...", map[string]any{"Count": totalQueued.Value()}))
				}
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
		// Pages are now persistent per session or until Nightly Refresh.

		// Ensure any blocked fetches are aborted so the new cycle can begin
		wp.CancelFetchContext()

		// Robust Sync: Reconcile store and invalidate stale derivatives
		wp.syncStoreWithConfig()

		// Ensure tray menu is rebuilt (e.g. if Favorites was toggled)
		if wp.manager != nil {
			wp.manager.RebuildTrayMenu()
		}

		// Trigger immediate fetch (Forced because we just applied settings)
		wp.FetchNewImages(true)

		// Wait for images using event driven notification (up to 15s)
		ctx, cancel := context.WithTimeout(wp.ctx, 15*time.Second)
		defer cancel()

		log.Debugf("[Init] Waiting for images before initial pulse...")
		if err := wp.store.WaitForImages(ctx); err == nil {
			log.Debugf("[Init] Images available. Triggering initial pulse.")
			// Use dispatch directly to bypass Stagger logic (Force Immediate)
			wp.dispatch(-1, CmdNext)
		} else {
			log.Println("[Init] Initial pulse timeout. Triggering anyway.")
			wp.dispatch(-1, CmdNext)
		}
	}()
}

func (wp *Plugin) fetchFromProvider(fetchCtx context.Context, q ImageQuery, p provider.ImageProvider, isFavRequest bool, sourcesMutex *sync.Mutex, activeSources map[string]bool, totalQueued *util.SafeCounter) {
	// Get or create per-query page counter
	wp.downloadMutex.Lock()
	pg, ok := wp.queryPages[q.ID]
	if !ok {
		pg = util.NewSafeIntWithValue(1)
		wp.queryPages[q.ID] = pg
	}
	wp.downloadMutex.Unlock()

	page := pg.Value()
	log.Debugf("Fetching from provider: %s (Query: %s, Page: %d)", q.Provider, q.Description, page)

	// Add timeout to prevent hangs
	ctx, cancel := context.WithTimeout(fetchCtx, 30*time.Second)
	defer cancel()

	// Rate limit API calls via PacedProvider interface
	// Pattern updated: Pacing penalty is now paid BEFORE grabbing the global semaphore in FetchNewImages.

	images, err := p.FetchImages(ctx, q.URL, page)
	if err != nil {
		log.Printf("Provider %s fetch failed: %v", q.Provider, err)
		return
	}
	if len(images) == 0 {
		log.Printf("Provider %s returned no new images for query %s.", q.Provider, q.ID)
		// Safe Page Wrapping: Auto-reset to page 1 for all providers to ensure they cycle through content correctly.
		// This ensures that "Nightly Refresh" and "Starvation Fetches" eventually see new content or loop back to favorites.
		if page > 1 {
			pg.Set(1)
			wp.saveQueryPages()
			log.Debugf("Query %s: End of content reached on page %d. Wrapping back to page 1.", q.ID, page)
		}
		return
	}

	log.Debugf("[Fetch] Provider %s returned %d images. Submitting to pipeline.", q.Provider, len(images))

	// Track source
	sourcesMutex.Lock()
	activeSources[p.ID()] = true
	sourcesMutex.Unlock()

	queuedForThisQuery := 0

	// Instantiate the background cancellation context for this specific query
	queryCtx := wp.GetOrCreateQueryContext(q.ID)

	for _, img := range images {
		// Critical Fix: Tag image with its source query ID so Sync knows it's active.
		img.SourceQueryID = q.ID

		// *** NAMESPACING Middleware ***
		// Ensure ID is unique across providers by prefixing it.
		if p.Type() == provider.TypeOnline {
			prefix := p.ID() + "_"
			if !strings.HasPrefix(img.ID, prefix) {
				img.ID = prefix + img.ID
			}
		}

		if !isFavRequest && wp.cfg.InAvoidSet(img.ID) {
			log.Debugf("Skipping blocked image: %s", img.ID)
			continue
		}

		// Pattern: Deduplication (At-the-Gate)
		// Deadlock Break: We only skip an image if it exists AND already has derivatives for your current monitors.
		// This allow "Backlog Healing": if you have 1000 images but 0 derivatives, this allows them back into the pipeline.
		if existing, exists := wp.store.GetByID(img.ID); exists {
			if wp.allMonitorDerivativesExist(existing) {
				log.Debugf("Skipping image already in store with all derivatives: %s", img.ID)
				continue
			}
			log.Debugf("Image %s exists but is missing derivatives. Allowing re-processing for backlog healing.", img.ID)
			// Merge existing metadata (like already probed dimensions) into the fetch-result image
			img.Width = existing.Width
			img.Height = existing.Height
			if img.ProcessingFlags == nil {
				img.ProcessingFlags = make(map[string]bool)
			}
			for k, v := range existing.ProcessingFlags {
				img.ProcessingFlags[k] = v
			}
		}
		job := DownloadJob{
			Ctx:      queryCtx,
			Image:    img,
			Provider: p,
		}
		// Submit blocking (until buffer clears or fetchCtx aborts)
		if wp.jobSubmitter.Submit(fetchCtx, job) {
			totalQueued.Increment()
			queuedForThisQuery++
		} else {
			// If submit returns false, the pipeline stopped or the fetch cycle was aborted via fetchCtx.
			// Do not attempt to process the rest of the images, and do NOT advance the pagination!
			log.Printf("Fetch aborted or pipeline full. Dropping job and pausing fetch for query %s.", q.ID)
			break
		}
	}

	if len(images) > 0 {
		pg.Increment()
		wp.saveQueryPages() // Persist pagination state
		log.Debugf("Query %s: Successfully processed page %d (Found: %d, Queued: %d). Incrementing to page %d", q.ID, page, len(images), queuedForThisQuery, pg.Value())
	}
}

// allMonitorDerivativesExist checks if the image has a processed file for every unique monitor resolution.
func (wp *Plugin) allMonitorDerivativesExist(img provider.Image) bool {
	if len(img.DerivativePaths) == 0 {
		return false
	}

	resolutions := wp.getResolutionsForDerivatives()
	if len(resolutions) == 0 {
		return true // No monitors to satisfy
	}

	for _, res := range resolutions {
		resKey := fmt.Sprintf("%dx%d", res.Width, res.Height)
		if _, ok := img.DerivativePaths[resKey]; !ok {
			// Also check incompatibility tag correctly
			tagKey := "incompatible:" + resKey
			if img.ProcessingFlags[tagKey] {
				continue // Already verified incompatible for this resolution
			}
			return false
		}
	}

	return true
}
