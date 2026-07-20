package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/curation"
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
	return hex.EncodeToString(hash[:])
}

type galleryLink struct {
	Name string
	Path string
}

func generateIndexHTML(dir string, galleries map[string][]galleryLink) {
	var builder strings.Builder
	builder.WriteString(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Spice &#127798;</title>
    <style>
        body { font-family: sans-serif; background: #2b2b2b; color: #eee; padding: 2rem; max-width: 800px; margin: 0 auto; }
        h1 { color: #f1c40f; }
        h2 { border-bottom: 1px solid #555; padding-bottom: 0.5rem; margin-top: 2rem; }
        ul { list-style-type: none; padding: 0; }
        li { margin-bottom: 0.5rem; }
        a { color: #3498db; text-decoration: none; font-size: 1.1rem; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <h1>Spice &#127798;</h1>
`)

	for museum, links := range galleries {
		builder.WriteString(fmt.Sprintf("    <h2>%s</h2>\n    <ul>\n", museum))
		for _, link := range links {
			builder.WriteString(fmt.Sprintf(`        <li><a href="%s">%s</a></li>`+"\n", link.Path, link.Name))
		}
		builder.WriteString("    </ul>\n")
	}

	builder.WriteString(`</body>
</html>`)

	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(builder.String()), 0600); err != nil {
		fmt.Printf("Failed to write index.html: %v\n", err)
	}
}

func main() {
	debugFlag := flag.Bool("debug", false, "Enable debug logging")
	fullFlag := flag.Bool("full", false, "Generate full galleries and index.html")
	limitFlag := flag.Int("limit", 8, "Maximum number of items per gallery (ignored if --full is used)")
	outFlag := flag.String("out", "", "Output directory (mandatory if --full is used)")
	flag.Parse()

	if *fullFlag && *outFlag == "" {
		fmt.Println("Error: --out is mandatory when using --full to prevent clobbering embedded app galleries.")
		os.Exit(1)
	}

	log.SetDebugEnabled(*debugFlag)

	ctx, cancel := context.WithTimeout(context.Background(), 15*3600*time.Second) // Allowing longer for large tasks
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
	if *outFlag != "" {
		if filepath.IsAbs(*outFlag) {
			baseDestDir = *outFlag
		} else {
			baseDestDir = filepath.Join(root, *outFlag)
		}
	}
	if err := os.MkdirAll(baseDestDir, 0755); err != nil {
		fmt.Printf("Error creating destination directory: %v\n", err)
		os.Exit(1)
	}

	hashStatePath := filepath.Join(baseDestDir, ".hash_state.json")
	hashState := make(map[string]string)
	if b, err := os.ReadFile(hashStatePath); err == nil {
		_ = json.Unmarshal(b, &hashState)
	}

	museumGalleries := make(map[string][]galleryLink)
	cfg := &wallpaper.Config{}
	client := &http.Client{Timeout: 30 * time.Second}

	var providers []provider.ImageProvider
	for _, factory := range wallpaper.GetRegisteredProviders() {
		providers = append(providers, factory(cfg, client))
	}

	for _, prov := range providers {
		if _, ok := prov.(provider.ThumbnailProvider); !ok {
			fmt.Printf("%s: Provider does not support thumbnails, skipping\n", prov.ID())
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
			if len(entry.IDs) == 0 && len(entry.Items) == 0 {
				continue
			}

			safeName := strings.ReplaceAll(strings.ToLower(entry.Key), " ", "_")
			fileName := fmt.Sprintf("%s.html", safeName)
			outPath := filepath.Join(destDir, fileName)
			stateKey := fmt.Sprintf("%s/%s", prov.ID(), fileName)

			link := galleryLink{
				Name: entry.Name,
				Path: fmt.Sprintf("%s/%s", strings.ToLower(prov.ID()), fileName),
			}
			museumGalleries[prov.ID()] = append(museumGalleries[prov.ID()], link)

			entryHash := calculateHash(entry)
			if existingHash, ok := hashState[stateKey]; ok && existingHash == entryHash {
				if _, err := os.Stat(outPath); err == nil {
					fmt.Printf("%s: Skipping %s (cached)\n", prov.ID(), fileName)
					continue
				}
			}

			limit := *limitFlag
			if *fullFlag {
				limit = 0
			}

			err := wallpaper.GenerateGalleryForProvider(ctx, prov, entry, cfg, &http.Client{}, destDir, limit)
			if err != nil {
				fmt.Printf("%s: Failed to generate gallery for %s: %v\n", prov.ID(), entry.Name, err)
			}
		}
	}

	// Save Cache State
	stateBytes, _ := json.MarshalIndent(hashState, "", "  ")
	if err := os.WriteFile(hashStatePath, stateBytes, 0600); err != nil {
		fmt.Printf("Warning: failed to write hash state: %v\n", err)
	}

	if *fullFlag {
		fmt.Println("Generating index.html...")
		generateIndexHTML(baseDestDir, museumGalleries)
	}

	fmt.Println("Done!")
}
