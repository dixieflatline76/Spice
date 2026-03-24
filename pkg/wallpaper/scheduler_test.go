package wallpaper

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util"
	"github.com/stretchr/testify/assert"
)

func TestCheckAndRunRefresh_Logic(t *testing.T) {
	tempDir := t.TempDir()

	wp := &Plugin{
		cfg:                GetConfig(NewMockPreferences()),
		fetchingInProgress: util.NewSafeBool(),
		queryPages:         make(map[string]*util.SafeCounter),
		providers:          make(map[string]provider.ImageProvider),
		httpClient:         &http.Client{Timeout: 10 * time.Millisecond}, // Fail fast
		store:              NewImageStore(),
		fm:                 NewFileManager(tempDir),
	}

	// Setup Store for Maintenance
	cachePath := filepath.Join(tempDir, "image_cache_map.json")
	wp.store.SetFileManager(wp.fm, cachePath)

	// Initially, it should NOT run unless it's midnight
	t1 := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	lastDay := wp.checkAndRunRefresh(t1, -1, true)
	assert.Equal(t, 24, lastDay)

	// Day change to 25
	t2 := time.Date(2026, 3, 25, 0, 1, 0, 0, time.UTC)

	// We want to verify that when it runs, it doesn't crash
	// and that it preserves our queryPages.
	wp.queryPages["q1"] = util.NewSafeIntWithValue(10)

	// We expect this might try to hit the network for connectivity check,
	// which will likely fail or timeout.
	fmt.Println("Running refresh check with simulated day change...")
	newLastDay := wp.checkAndRunRefresh(t2, lastDay, false)

	// If network is unavailable (likely in test), it won't run but will return the same lastDay?
	// No, if shouldRun is true, it tries to run. If network fails, it returns the OLD lastDay.

	// Let's check if our page counter is Still 10 (it should be regardless of whether it ran or failed)
	assert.Equal(t, 10, wp.queryPages["q1"].Value(), "Query page should never be reset in refresh")

	if newLastDay == 25 {
		fmt.Println("Refresh successfully triggered and completed (or bypassed network check).")
	} else {
		fmt.Println("Refresh skipped (likely network check failed).")
	}
}
