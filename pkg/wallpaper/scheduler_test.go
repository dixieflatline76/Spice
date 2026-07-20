package wallpaper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/curation"

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

func TestCheckAndRunRefresh_OTAIntegration(t *testing.T) {
	tempDir := t.TempDir()

	wp := &Plugin{
		cfg:                GetConfig(NewMockPreferences()),
		fetchingInProgress: util.NewSafeBool(),
		queryPages:         make(map[string]*util.SafeCounter),
		providers:          make(map[string]provider.ImageProvider),
		httpClient: &http.Client{
			Timeout:   10 * time.Millisecond,
			Transport: &mockTransport{},
		},
		store: NewImageStore(),
		fm:    NewFileManager(tempDir),
	}

	// Enable OTA in config
	wp.cfg.SetMuseumCollectionOTA(true)

	// Set up mock remote server for Curation Manager
	remoteJSON := `{
		"version": "v9.9.9",
		"description": "Mock Remote Collection",
		"collections": []
	}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(remoteJSON))
	}))
	defer ts.Close()

	cm := curation.GetManager()
	cm.RemoteBaseURL = ts.URL + "/"
	cm.CacheDir = filepath.Join(tempDir, "curation_cache")

	// Create a dummy provider so SyncAll triggers an update
	curation.ProviderIDToFilename["testprov"] = "testprov.json"

	// Fast forward time to trigger a new day refresh
	now := time.Now()
	lastRefreshDay := now.Add(-48 * time.Hour).Day()

	newRefreshDay := wp.checkAndRunRefresh(now, lastRefreshDay, true)

	assert.Equal(t, now.Day(), newRefreshDay, "Expected refresh day to update")

	// Wait briefly to allow background SyncAll and generation workers to run
	time.Sleep(500 * time.Millisecond)

	// Verify OTA was actually triggered by checking if the cache file was written
	cachePath := filepath.Join(cm.CacheDir, "testprov.json")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Errorf("Expected OTA curation cache file to be written at %s", cachePath)
	}
}

type mockTransport struct{}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 204,
		Body:       http.NoBody,
	}, nil
}
