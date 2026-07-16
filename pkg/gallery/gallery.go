package gallery

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dixieflatline76/Spice/v2/asset/galleries"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// UnpackAll extracts all pre-generated HTML galleries from the embedded binary assets
// into the user's local working cache directory. It unpacks them into provider-specific
// subdirectories (e.g. cache/getty/).
func UnpackAll(cacheRootDir string) error {
	providers, err := fs.ReadDir(galleries.EmbeddedGalleries, ".")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read embedded galleries root: %w", err)
	}

	var execModTime time.Time
	if execPath, err := os.Executable(); err == nil {
		if stat, err := os.Stat(execPath); err == nil {
			execModTime = stat.ModTime()
		}
	}

	for _, pEntry := range providers {
		if !pEntry.IsDir() {
			continue
		}

		providerID := pEntry.Name()
		// Convert to lowercase to match the existing cache folder structure (e.g. "Getty" -> "getty")
		providerCacheDir := filepath.Join(cacheRootDir, strings.ToLower(providerID))
		if err := os.MkdirAll(providerCacheDir, 0755); err != nil {
			log.Printf("Gallery: Failed to create cache dir for %s: %v", providerID, err)
			continue
		}

		entries, err := fs.ReadDir(galleries.EmbeddedGalleries, providerID)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".html" {
				continue
			}

			assetPath := filepath.Join(providerID, entry.Name())
			data, err := galleries.EmbeddedGalleries.ReadFile(filepath.ToSlash(assetPath))
			if err != nil {
				log.Printf("Gallery: Failed to read embedded file %s: %v", assetPath, err)
				continue
			}

			outPath := filepath.Join(providerCacheDir, entry.Name())

			shouldWrite := false
			if stat, err := os.Stat(outPath); os.IsNotExist(err) {
				shouldWrite = true
			} else if err == nil && !execModTime.IsZero() && stat.ModTime().Before(execModTime) {
				// The cached file is older than the current application binary.
				// This means the embedded file is from a newer version of the app.
				shouldWrite = true
			}

			if shouldWrite {
				if err := os.WriteFile(outPath, data, 0600); err != nil {
					log.Printf("Gallery: Failed to unpack %s to %s: %v", entry.Name(), outPath, err)
				}
			}
		}
	}
	return nil
}

// GenerateHTML takes a collection title and image URLs and returns
// a beautiful virtual gallery wall HTML string.
func GenerateHTML(title string, imageUrls []string) (string, error) {
	tmplStr := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Spice Gallery</title>
    <style>
        body {
            background-color: #4a3b2c;
            margin: 0;
            padding: 60px 40px;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            font-family: system-ui, -apple-system, sans-serif;
        }

        h1 {
            color: rgba(255, 255, 255, 0.9);
            font-weight: 300;
            letter-spacing: 2px;
            margin-bottom: 60px;
            text-shadow: 0 4px 10px rgba(0,0,0,0.5);
            text-align: center;
        }

        .gallery-container {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 40px;
            max-width: 1600px;
            width: 100%;
            align-items: center;
        }

        .frame {
            background: linear-gradient(135deg, #4a2f1d, #2d1c11);
            padding: 12px;
            border-radius: 4px;
            box-shadow: 0 30px 60px rgba(0, 0, 0, 0.6), 0 10px 20px rgba(0, 0, 0, 0.4);
            transition: transform 0.3s ease;
            position: relative;
        }

        .frame:hover {
            transform: scale(1.02) translateY(-5px);
        }

        .matboard {
            background-color: #fdfdfa;
            padding: 25px;
            box-shadow: inset 0 0 10px rgba(0,0,0,0.5);
            display: inline-block;
        }

        .artwork {
            max-width: 100%;
            height: auto;
            max-height: 300px;
            display: block;
            box-shadow: 0 2px 5px rgba(0,0,0,0.4);
        }

        /* Staggered masonry offsets */
        .frame:nth-child(even) {
            transform: translateY(40px);
        }
        .frame:nth-child(even):hover {
            transform: scale(1.02) translateY(35px);
        }
    </style>
</head>
<body>
    <h1>{{.Title}}</h1>
    <div class="gallery-container">
        {{range .Images}}
        <div class="frame">
            <div class="matboard">
                <img class="artwork" src="{{.}}" />
            </div>
        </div>
        {{end}}
    </div>
</body>
</html>`

	t, err := template.New("gallery").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse gallery template: %w", err)
	}

	safeImages := make([]template.URL, len(imageUrls))
	for i, u := range imageUrls {
		//nolint:gosec // These URLs are internally generated base64 strings or trusted provider thumbnails
		safeImages[i] = template.URL(u)
	}

	data := struct {
		Title  string
		Images []template.URL
	}{
		Title:  title,
		Images: safeImages,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute gallery template: %w", err)
	}

	return buf.String(), nil
}
