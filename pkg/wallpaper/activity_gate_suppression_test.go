package wallpaper

import (
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/util"
	"github.com/stretchr/testify/assert"
)

// TestRequestFetch_GateSuppression verifies step 2.12 of the implementation plan:
// When gateSuppressed is true, RequestFetch returns immediately and never spawns
// background fetch activity (even for targeted or untargeted requests).
// Once gateSuppressed is false, RequestFetch proceeds normally.
func TestRequestFetch_GateSuppression(t *testing.T) {
	wp := &Plugin{
		gateSuppressed:     util.NewSafeBool(),
		fetchingInProgress: util.NewSafeBool(),
		interrupt:          util.NewSafeBool(),
		store:              NewImageStore(),
	}

	// 1. Suppressed state: RequestFetch must be a no-op
	wp.gateSuppressed.Set(true)

	initialTime := wp.lastTriggerTime
	wp.RequestFetch()

	assert.Equal(t, initialTime, wp.lastTriggerTime, "lastTriggerTime must not change while gate-suppressed")
	assert.False(t, wp.isDownloading, "must not initiate download while gate-suppressed")

	// Even targeted fetches (e.g. "Favorites") must be blocked during game/battery suppression
	wp.RequestFetch("Favorites")
	assert.False(t, wp.isDownloading)

	// 2. Unsuppressed state: RequestFetch proceeds past the suppression gate
	wp.gateSuppressed.Set(false)
	wp.RequestFetch()
	assert.True(t, wp.lastTriggerTime.After(initialTime),
		"RequestFetch must update lastTriggerTime when unsuppressed")

	// Allow any goroutine spawned by the unsuppressed path to settle
	time.Sleep(10 * time.Millisecond)
}
