package wallpaper

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dixieflatline76/Spice/util/log"
)

// startNightlyRefresher runs a goroutine that periodically checks if a nightly refresh is due.
func (wp *Plugin) startNightlyRefresher() {
	log.Print("Starting nightly refresh checker...")

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	var lastRefreshDay int = -1 // Initialize to -1 to ensure the first check works correctly

	runCheckWithTimeout := func(now time.Time, lastDay int, isStartup bool) int {
		done := make(chan int)
		timeoutDuration := 5 * time.Minute

		go func() {
			result := wp.checkAndRunRefresh(now, lastDay, isStartup)
			select {
			case done <- result:
			default:
				log.Print("checkAndRunRefresh completed, but the call had already timed out.")
			}
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
		log.Printf("Initial refresh check at %s", now.Format(time.RFC3339))

		if lastRefreshDay == -1 && now.Hour() == 0 && now.Minute() < 6 {
			shouldRun = true
			reason = "Initial check detected start/wake-up shortly after midnight."
		} else if lastRefreshDay == -1 {
			reason = fmt.Sprintf("Initial check: Current time (%s) is not post-midnight. Setting last refresh day to %d.", now.Format(time.Kitchen), today)
			log.Print(reason)
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
		log.Printf("Decision: Refresh needed. Reason: %s", reason) // Log why it's running

		// Network Check
		if !wp.isNetworkAvailable() {
			log.Print("Nightly refresh check: Network appears to be unavailable. Skipping refresh cycle.")
			return lastRefreshDay
		}
		log.Print("Nightly refresh check: Network available. Proceeding with refresh...")

		updatedLastRefreshDay := today

		log.Print("Running nightly refresh action...") // Clarify log message
		wp.currentDownloadPage.Set(1)
		wp.downloadAllImages(nil) // This calls stopAllWorkers internally

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
		log.Printf("isNetworkAvailable: Network check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}

	log.Printf("isNetworkAvailable: Network check returned non-success status: %d", resp.StatusCode)
	return false
}
