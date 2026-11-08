package wallhaven

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLiveRateLimit(t *testing.T) {
	if os.Getenv("SPICE_LIVE_TESTS") == "" {
		t.Skip("Skipping live network test in CI. Run with SPICE_LIVE_TESTS=1 to execute.")
	}

	apiURL := "https://wallhaven.cc/api/v1/search?q=anime&sorting=random"
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(apiURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Print Rate Limit Headers
	t.Logf("Status: %s", resp.Status)
	t.Logf("X-Rate-Limit-Limit: %s", resp.Header.Get("X-Rate-Limit-Limit"))
	t.Logf("X-Rate-Limit-Remaining: %s", resp.Header.Get("X-Rate-Limit-Remaining"))
	t.Logf("X-Rate-Limit-Reset: %s", resp.Header.Get("X-Rate-Limit-Reset"))

	if resp.StatusCode == 429 {
		t.Error("Rate Limit Hit!")
	}
}
