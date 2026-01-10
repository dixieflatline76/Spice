package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type ArticCollection struct {
	Tours map[string]struct {
		IDs []int `json:"ids"`
	} `json:"tours"`
}

type ArtData struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	IsPublicDomain bool   `json:"is_public_domain"`
	ImageID        string `json:"image_id"`
	Thumbnail      struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"thumbnail"`
}

type Response struct {
	Data ArtData `json:"data"`
}

func getSourcePath() string {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "../../..")
	return filepath.Join(root, "pkg/wallpaper/providers/artic/artic.json")
}

func main() {
	jsonPath := getSourcePath()
	f, err := os.Open(jsonPath)
	if err != nil {
		fmt.Printf("Failed to open artic.json at %s: %v\n", jsonPath, err)
		os.Exit(1)
	}
	defer f.Close()

	var coll ArticCollection
	if err := json.NewDecoder(f).Decode(&coll); err != nil {
		fmt.Printf("Failed to decode artic.json: %v\n", err)
		os.Exit(1)
	}

	var ids []int
	for _, tour := range coll.Tours {
		ids = append(ids, tour.IDs...)
	}

	fmt.Printf("Auditing %d items from %s...\n", len(ids), jsonPath)

	var wg sync.WaitGroup
	results := make([]string, len(ids))

	fmt.Println("ID | Status | Ratio | PublicDomain | Title")
	fmt.Println("---|---|---|---|---")

	for i, id := range ids {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}
			url := fmt.Sprintf("https://api.artic.edu/api/v1/artworks/%d?fields=id,title,artist_display,is_public_domain,image_id,thumbnail", id)

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

			var data Response
			if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
				results[i] = fmt.Sprintf("%d | JSON Err | - | - | %v", id, err)
				return
			}

			d := data.Data
			if d.Thumbnail.Height == 0 {
				results[i] = fmt.Sprintf("%d | NO_DATA | - | %v | %s", d.ID, d.IsPublicDomain, d.Title)
				return
			}

			ratio := float64(d.Thumbnail.Width) / float64(d.Thumbnail.Height)
			status := "OK"
			if ratio < 1.1 {
				status = "PORTRAIT"
			}
			if !d.IsPublicDomain {
				status = "COPYRIGHT"
			}
			if d.ImageID == "" {
				status = "NO_IMAGE"
			}

			results[i] = fmt.Sprintf("%d | %s | %.2f | %v | %s", d.ID, status, ratio, d.IsPublicDomain, d.Title)
		}(i, id)
	}

	wg.Wait()
	for _, r := range results {
		fmt.Println(r)
	}
}
