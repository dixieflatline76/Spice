package main

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type MetCollection struct {
	IDs []int `json:"ids"`
}

type ArtData struct {
	ObjectID       int    `json:"objectID"`
	Title          string `json:"title"`
	Artist         string `json:"artistDisplayName"`
	IsPublicDomain bool   `json:"isPublicDomain"`
	PrimaryImage   string `json:"primaryImage"`
}

func getSourcePath() string {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "../../..")
	return filepath.Join(root, "pkg/wallpaper/providers/metmuseum/met.json")
}

func main() {
	jsonPath := getSourcePath()
	f, err := os.Open(jsonPath)
	if err != nil {
		fmt.Printf("Failed to open met.json at %s: %v\n", jsonPath, err)
		os.Exit(1)
	}
	defer f.Close()

	var coll MetCollection
	if err := json.NewDecoder(f).Decode(&coll); err != nil {
		fmt.Printf("Failed to decode met.json: %v\n", err)
		os.Exit(1)
	}

	ids := coll.IDs
	fmt.Printf("Auditing %d items from %s...\n", len(ids), jsonPath)

	var wg sync.WaitGroup
	results := make([]string, len(ids))

	fmt.Println("ID | Status | Ratio | PublicDomain | Title")
	fmt.Println("---|---|---|---|---")

	for i, id := range ids {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			client := &http.Client{Timeout: 10 * time.Second}
			url := fmt.Sprintf("https://collectionapi.metmuseum.org/public/collection/v1/objects/%d", id)

			resp, err := client.Get(url)
			if err != nil {
				results[i] = fmt.Sprintf("%d | ERROR | - | - | %v", id, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				results[i] = fmt.Sprintf("%d | HTTP %d | - | - | Failed", id, resp.StatusCode)
				return
			}

			var d ArtData
			if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
				results[i] = fmt.Sprintf("%d | JSON Err | - | - | %v", id, err)
				return
			}

			if !d.IsPublicDomain {
				results[i] = fmt.Sprintf("%d | COPYRIGHT | - | false | %s", d.ObjectID, d.Title)
				return
			}

			if d.PrimaryImage == "" {
				results[i] = fmt.Sprintf("%d | NO_IMAGE | - | true | %s", d.ObjectID, d.Title)
				return
			}

			// Fetch image header for dimensions
			imgResp, err := client.Get(d.PrimaryImage)
			if err != nil {
				results[i] = fmt.Sprintf("%d | IMG ERR | - | true | %s", d.ObjectID, d.Title)
				return
			}
			defer imgResp.Body.Close()

			cfg, _, err := image.DecodeConfig(imgResp.Body)
			if err != nil {
				results[i] = fmt.Sprintf("%d | DECODE ERR | - | true | %s", d.ObjectID, d.Title)
				return
			}

			ratio := float64(cfg.Width) / float64(cfg.Height)
			status := "OK"
			if ratio < 1.1 {
				status = "PORTRAIT"
			}

			results[i] = fmt.Sprintf("%d | %s | %.2f | true | %s", d.ObjectID, status, ratio, d.Title)
		}(i, id)
	}

	wg.Wait()
	for _, r := range results {
		fmt.Println(r)
	}
}
