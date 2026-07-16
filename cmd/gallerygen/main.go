package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	_ "image/gif"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/curation"
	"github.com/dixieflatline76/Spice/v2/pkg/gallery"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"

	// Import providers to register them
	_ "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/artic"
	_ "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/cleveland"
	_ "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/getty"
	_ "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/metmuseum"
	_ "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/npm"
	_ "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/rijksmuseum"
	_ "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/smk"
)


func calculateHash(entry curation.CollectionEntry) string {
	b, _ := json.Marshal(entry)
	hash := sha256.Sum256(b)
	return fmt.Sprintf("%x", hash)
}

func main() {
	debugFlag := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	log.SetDebugEnabled(*debugFlag)

	// Generating galleries for all museums can take several minutes due to API rate limiting
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	fmt.Println("Starting Virtual Gallery Generator...")

	wd, _ := os.Getwd()
	root := wd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			fmt.Println("Error: Could not find go.mod in any parent directory.")
			os.Exit(1)
		}
		root = parent
	}

	baseDestDir := filepath.Join(root, "asset", "galleries")
	if err := os.MkdirAll(baseDestDir, 0755); err != nil {
		fmt.Printf("Error creating destination directory: %v\n", err)
		os.Exit(1)
	}

	// Load Cache State
	hashStatePath := filepath.Join(baseDestDir, ".hash_state.json")
	hashState := make(map[string]string)
	if b, err := os.ReadFile(hashStatePath); err == nil {
		if err := json.Unmarshal(b, &hashState); err != nil {
			fmt.Printf("Warning: failed to unmarshal hash state: %v\n", err)
		}
	}

	cfg := &wallpaper.Config{}
	client := &http.Client{Timeout: 15 * time.Second}

	time.Sleep(500 * time.Millisecond)

	var providers []provider.ImageProvider
	for _, factory := range wallpaper.GetRegisteredProviders() {
		providers = append(providers, factory(cfg, client))
	}

	for _, prov := range providers {
		thumbProv, ok := prov.(provider.ThumbnailProvider)
		if !ok {
			continue
		}

		col := curation.GetManager().GetCollection(prov.ID())
		if col == nil {
			fmt.Printf("Skipping %s: curated list not loaded\n", prov.ID())
			continue
		}

		destDir := filepath.Join(baseDestDir, strings.ToLower(prov.ID()))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			fmt.Printf("Skipping %s: failed to create dest dir: %v\n", prov.ID(), err)
			continue
		}

		fmt.Printf("Generating galleries for %s...\n", prov.ID())

		for _, entry := range col.Entries {
			safeName := strings.ReplaceAll(strings.ToLower(entry.Key), " ", "_")
			fileName := fmt.Sprintf("%s.html", safeName)
			outPath := filepath.Join(destDir, fileName)
			stateKey := fmt.Sprintf("%s/%s", prov.ID(), fileName)

			entryHash := calculateHash(entry)
			if existingHash, ok := hashState[stateKey]; ok && existingHash == entryHash {
				if _, err := os.Stat(outPath); err == nil {
					fmt.Printf("%s: Skipping %s (cached)\n", prov.ID(), fileName)
					continue
				}
			}

			count := 8

			var idsToFetch []string
			if len(entry.IDs) > 0 {
				if len(entry.IDs) < count {
					count = len(entry.IDs)
				}
				idsToFetch = entry.IDs[:count]
			} else if len(entry.Items) > 0 {
				if len(entry.Items) < count {
					count = len(entry.Items)
				}
				for _, item := range entry.Items[:count] {
					b, _ := json.Marshal(item)
					idsToFetch = append(idsToFetch, string(b))
				}
			}

			if len(idsToFetch) == 0 {
				continue
			}

			thumbnails, err := thumbProv.FetchThumbnails(ctx, idsToFetch)
			if err != nil {
				fmt.Printf("%s: Failed to fetch thumbnails for %s: %v\n", prov.ID(), entry.Name, err)
				continue
			}

			var wg sync.WaitGroup
			urls := make([]string, len(thumbnails))
			for i, t := range thumbnails {
				wg.Add(1)
				go func(index int, thumb provider.Thumbnail) {
					defer wg.Done()
					req, err := http.NewRequestWithContext(ctx, "GET", thumb.URL, nil)
					if err != nil {
						urls[index] = thumb.URL
						return
					}

					if hp, ok := prov.(provider.HeaderProvider); ok {
						for k, v := range hp.GetDownloadHeaders() {
							req.Header.Set(k, v)
						}
					}

					resp, err := client.Do(req)
					if err == nil && resp.StatusCode == 200 {
						b, err := io.ReadAll(resp.Body)
						if err == nil {
							b64 := base64.StdEncoding.EncodeToString(b)
							ct := resp.Header.Get("Content-Type")
							if ct == "" {
								ct = "image/jpeg"
							}
							urls[index] = fmt.Sprintf("data:%s;base64,%s", ct, b64)
						} else {
							urls[index] = thumb.URL
						}
					} else {
						urls[index] = thumb.URL
					}
					if resp != nil && resp.Body != nil {
						resp.Body.Close()
					}
				}(i, t)
			}
			wg.Wait()

			var validUrls []string
			for _, u := range urls {
				if u != "" {
					validUrls = append(validUrls, u)
				}
			}

			if len(validUrls) == 0 {
				continue
			}

			htmlContent, err := gallery.GenerateHTML(entry.Name, validUrls)
			if err != nil {
				fmt.Printf("%s: Failed to generate HTML for %s: %v\n", prov.ID(), entry.Name, err)
				continue
			}

			if err := os.WriteFile(outPath, []byte(htmlContent), 0600); err != nil {
				fmt.Printf("%s: Failed to write HTML for %s: %v\n", prov.ID(), entry.Name, err)
			} else {
				fmt.Printf("%s: Generated gallery %s\n", prov.ID(), fileName)
				hashState[stateKey] = entryHash
			}
		}
	}

	// Save Cache State
	stateBytes, _ := json.MarshalIndent(hashState, "", "  ")
	if err := os.WriteFile(hashStatePath, stateBytes, 0600); err != nil {
		fmt.Printf("Warning: failed to write hash state: %v\n", err)
	}

	fmt.Println("Done!")
}
