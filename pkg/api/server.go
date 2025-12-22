package api

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Server represents the Local REST/WebSocket server.
type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
	upgrader   websocket.Upgrader

	// WebSocket management
	clients   map[*websocket.Conn]bool
	clientsMu sync.Mutex

	// Local file serving
	namespaces map[string]string // name -> absPath

	// Callbacks
	onAddCollection func(url string) error
}

// NewServer creates a new API server.
func NewServer() *Server {
	s := &Server{
		mux: http.NewServeMux(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		clients:    make(map[*websocket.Conn]bool),
		namespaces: make(map[string]string),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/health", s.enableCORS(s.handleHealth))
	s.mux.HandleFunc("/ws", s.handleWebSocket)
	s.mux.HandleFunc("/add", s.enableCORS(s.handleAdd))
	s.mux.HandleFunc("/local/", s.enableCORS(s.handleLocal))
}

// RegisterNamespace registers a local directory to be served under /local/{name}.
func (s *Server) RegisterNamespace(name, path string) {
	s.namespaces[name] = path
}

// enableCORS adds CORS headers to the handler.
func (s *Server) enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow extensions to access localhost
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// SetAddCollectionHandler sets the callback for adding a collection.
func (s *Server) SetAddCollectionHandler(handler func(url string) error) {
	s.onAddCollection = handler
}

// Handler returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Start starts the server.
func (s *Server) Start() error {
	s.httpServer = &http.Server{
		Addr:    "127.0.0.1:49452",
		Handler: s.mux,
	}
	// This is blocking
	return s.httpServer.ListenAndServe()
}

// Stop stops the server.
func (s *Server) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(context.Background())
	}
	return nil
}

// BroadcastWallpaper sends a "set_wallpaper" command to all connected clients.
func (s *Server) BroadcastWallpaper(path string) error {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	msg := map[string]string{
		"type": "set_wallpaper",
		"url":  path, // Using 'url' key to match extension expectation, though it's a path
	}

	for client := range s.clients {
		err := client.WriteJSON(msg)
		if err != nil {
			log.Printf("Failed to broadcast to client: %v", err)
			client.Close()
			delete(s.clients, client)
		}
	}
	return nil
}
