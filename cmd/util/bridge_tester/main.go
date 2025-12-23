package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Upgrade error:", err)
			return
		}
		defer conn.Close()

		log.Println("✅ Extension Connected!")

		// Listen for Pings
		go func() {
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					log.Println("Read error:", err)
					return
				}
				log.Printf("📩 Received from Extension: %s\n", msg)
			}
		}()

		// Send commands
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			cmd := map[string]string{
				"type": "set_wallpaper",
				"url":  "https://wallhaven.cc/fake_wallpaper.jpg",
			}
			if err := conn.WriteJSON(cmd); err != nil {
				log.Println("Write error:", err)
				return
			}
			log.Println("🚀 Sent 'set_wallpaper' command to Extension")
		}
	})

	log.Println("📡 Bridge Tester running on :49452. Waiting for Extension...")
	log.Println("📡 Bridge Tester running on :49452. Waiting for Extension...")
	server := &http.Server{
		Addr:              ":49452",
		Handler:           nil, // uses DefaultServeMux
		ReadHeaderTimeout: 3 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal("Server error:", err)
	}
}
