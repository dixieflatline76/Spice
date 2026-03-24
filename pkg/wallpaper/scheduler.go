package wallpaper

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dixieflatline76/Spice/v2/util/log"
)

// StartNightlyRefresh runs a goroutine that periodically checks if a nightly refresh is due.
func (wp *Plugin) StartNightlyRefresh() {
	wp.downloadMutex.Lock()
	if wp.stopNightlyRefresh == nil {
		wp.stopNightlyRefresh = make(chan struct{})
	}
	wp.downloadMutex.Unlock()

	log.Debugf("Starting nightly refresh checker...")

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	var lastRefreshDay int = -1 // Initialize to -1 to ensure the first check works correctly

	runCheckWithTimeout := func(now time.Time, lastDay int, isStartup bool) int {
		done := make(chan int, 1)
		timeoutDuration := 5 * time.Minute

		go func() {
			result := wp.checkAndRunRefresh(now, lastDay, isStartup)
			done <- result
		}()

		select {
		case res := <-done:
			return res
		case <-time.After(timeoutDuration):
			log.Printf("!!! HANG DETECTED !!! Timeout of %v reached while waiting for refresh check.", timeoutDuration)
			return lastDay
		}
	}

	initialTime := time.Now()
	// Trigger sync on startup if enabled
	wp.SyncWallhavenCollections()
	lastRefreshDay = runCheckWithTimeout(initialTime, lastRefreshDay, true) // Force check on startup

	for {
		select {
		case now := <-ticker.C:
			lastRefreshDay = runCheckWithTimeout(now, lastRefreshDay, false) // Normal periodic check
		case <-wp.stopNightlyRefresh:
			log.Print("Stopping nightly refresh checker.")
			return // Exit the goroutine
		}
	}
}

// checkAndRunRefresh determines if a nightly refresh should be performed based on the current day and time.
func (wp *Plugin) checkAndRunRefresh(now time.Time, lastRefreshDay int, isInitialCheck bool) int {
	today := now.Day()
	shouldRun := false
	reason := "" // For logging clarity

	if isInitialCheck {
		log.Debugf("Initial refresh check at %s", now.Format(time.RFC3339))

		if lastRefreshDay == -1 && now.Hour() == 0 && now.Minute() < 6 {
			shouldRun = true
			reason = "Initial check detected start/wake-up shortly after midnight."
		} else if lastRefreshDay == -1 {
			reason = fmt.Sprintf("Initial check: Current time (%s) is not post-midnight. Setting last refresh day to %d.", now.Format(time.Kitchen), today)
			log.Debugf("%s", reason)
			lastRefreshDay = today // IMPORTANT: Set lastRefreshDay here for non-midnight starts
		}
	}

	if today != lastRefreshDay {
		if !shouldRun {
			shouldRun = true
			reason = fmt.Sprintf("Detected day change (%d -> %d at %s).", lastRefreshDay, today, now.Format(time.RFC3339))
		}
	}

	if shouldRun {
		log.Debugf("Decision: Refresh needed. Reason: %s", reason)

		// Network Check
		if !wp.isNetworkAvailable() {
			log.Print("Nightly refresh check: Network appears to be unavailable. Skipping refresh cycle.")
			return lastRefreshDay
		}
		log.Print("Nightly refresh check: Network available. Proceeding with refresh...")

		updatedLastRefreshDay := today

		// Maintenance: Grooming & Cleanup
		log.Print("Nightly Maintenance: Starting cache grooming...")
		targetFlags := map[string]bool{
			"SmartFit": wp.cfg.GetSmartFit(),
			"FaceCrop": wp.cfg.GetFaceCropEnabled(),
		}
		// Sync Store (Groom old images)
		wp.store.Sync(int(wp.cfg.GetCacheSize().Size()), targetFlags, wp.cfg.GetActiveQueryIDs())

		// Cleanup Orphans (Delete unknown files)
		// We get known IDs from store (thread-safe)
		wp.fm.CleanupOrphans(wp.store.GetKnownIDs())
		log.Print("Nightly Maintenance: Finished.")

		// Wallhaven Sync
		wp.SyncWallhavenCollections()

		log.Print("Running nightly refresh action...") // Clarify log message
		// Forward-Scanning Logic: We no longer force a reset to Page 1 every night.
		// Instead, we let the system naturally "drift" forward.
		// Safe Page Wrapping (in fetch_logic.go) will handle looping back only when a query is exhausted.
		wp.FetchNewImages(false)

		log.Print("Nightly refresh action finished.")
		return updatedLastRefreshDay // Return the new day
	}

	return lastRefreshDay
}

// isNetworkAvailable checks if the device has a stable internet connection by attempting to connect to a public endpoint.
func (wp *Plugin) isNetworkAvailable() bool {
	checkURL := "https://connectivitycheck.gstatic.com/generate_204"

	ctx, cancel := context.WithTimeout(context.Background(), NetworkConnectivityCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, checkURL, nil)
	if err != nil {
		log.Printf("isNetworkAvailable: Error creating request: %v", err)
		return false
	}

	resp, err := wp.httpClient.Do(req)
	if err != nil {
		log.Debugf("isNetworkAvailable: Network check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}

	log.Debugf("isNetworkAvailable: Network check returned non-success status: %d", resp.StatusCode)
	return false
}
