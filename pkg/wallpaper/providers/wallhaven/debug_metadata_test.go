package wallhaven

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLiveRateLimit(t *testing.T) {
	apiURL := "https://wallhaven.cc/api/v1/search?q=anime&sorting=random"
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(apiURL)
	assert.NoError(t, err)
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
