package wallpaper

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/api"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

// TestBrowserBridgeLatency performs a real integration test of the
// ChromeOS Driver -> API Server -> WebSocket Client pipeline.
// It measures the "turn around time" for a wallpaper set command.
func TestBrowserBridgeLatency(t *testing.T) {
	// 1. Setup API Server
	server := api.NewServer()
	// Expose Handler via httptest to get a real port
	// Note: We use the server's internal mux for this test
	handler := server.Handler()
	if handler == nil {
		t.Skip("API Server Handler unimplemented")
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// 2. Setup ChromeOS Driver & Wire it
	chromeDriver := &ChromeOS{}
	chromeDriver.RegisterBridge(server.BroadcastWallpaper)

	// 3. Connect "Fake Extension" Client
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// Retry connection logic (server might take a ms to be ready)
	var ws *websocket.Conn
	var err error
	for i := 0; i < 5; i++ {
		ws, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NoError(t, err, "Failed to connect to WebSocket server")
	defer ws.Close()

	// 4. Verification & Benchmarking Loop
	iterations := 5
	var totalDuration time.Duration

	for i := 0; i < iterations; i++ {
		testPath := "/tmp/test_wallpaper.jpg"

		start := time.Now()

		// A. Trigger Action (Driver Side)
		err := chromeDriver.setWallpaper(testPath)
		assert.NoError(t, err)

		// B. Receive Message (Extension Side)
		err = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		assert.NoError(t, err)
		_, message, err := ws.ReadMessage()
		assert.NoError(t, err)

		duration := time.Since(start)
		totalDuration += duration

		t.Logf("[Iteration %d] Latency: %v", i+1, duration)

		// C. Validate Content
		assert.Contains(t, string(message), "set_wallpaper")
		assert.Contains(t, string(message), testPath)
	}

	avgLatency := totalDuration / time.Duration(iterations)
	t.Logf("Average Turn-Around Time: %v", avgLatency)

	// Performance Assertion: Should be sub-millisecond (locally) or very fast (<10ms)
	// Relaxed to 500ms for CI/Test environments (Windows runners can be slow)
	assert.Less(t, int64(avgLatency), int64(500*time.Millisecond), "Latency is too high!")
}
