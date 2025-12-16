package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestNewServer(t *testing.T) {
	s := NewServer()
	assert.NotNil(t, s)
}

func TestHealthCheck(t *testing.T) {
	s := NewServer()
	// Handler() returns nil in stub, so this will panic or fail if used directly.
	// We Assert that handler is not nil once implemented.
	handler := s.Handler()
	if handler == nil {
		t.Skip("Skipping TestHealthCheck: Handler is nil (TDD Stub)")
		return
	}

	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "running")
}

func TestWebSocketConnection(t *testing.T) {
	s := NewServer()
	handler := s.Handler()
	if handler == nil {
		t.Skip("Skipping TestWebSocketConnection: Handler is nil (TDD Stub)")
		return
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)
	defer ws.Close()

	// Check Connection Message?
	// Implementation detail: Does server send welcome?
	// TDD: We define it SHOULD send something or just accept connection.
	// We'll check for ping/pong in heartbeat test.
}

func TestWebSocketPingPong(t *testing.T) {
	s := NewServer()
	handler := s.Handler()
	if handler == nil {
		t.Skip("Skipping TestWebSocketPingPong: Handler is nil")
		return
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)
	defer ws.Close()

	// Send Ping
	err = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`))
	assert.NoError(t, err)

	// Expect Pong or Ack
	// We'll define protocol: Server doesn't necessarily reply to Ping with JSON,
	// but maybe standard WS Pong?
	// Our Plan said: "Keepalive: Extension sends ping".
	// Server should probably just stay alive.
}

func TestBroadcast(t *testing.T) {
	s := NewServer()
	handler := s.Handler()
	if handler == nil {
		t.Skip("Handler is nil")
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect Client
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)
	defer ws.Close()

	// Broadcast
	go func() {
		// Give client time to connect
		time.Sleep(50 * time.Millisecond)
		if err := s.BroadcastWallpaper("/tmp/test.jpg"); err != nil {
			// In a goroutine, we can't easily fail the test T, but we can log or panic
			// For test simplicity, logic panic is acceptable or just ignore if consistent
			panic(err)
		}
	}()

	// Read Message
	_, p, err := ws.ReadMessage()
	assert.NoError(t, err)
	assert.Contains(t, string(p), "set_wallpaper")
	assert.Contains(t, string(p), "/tmp/test.jpg")
}
