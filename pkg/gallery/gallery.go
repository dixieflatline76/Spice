package gallery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dixieflatline76/Spice/v2/asset/galleries"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
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

// GenerateHTML takes a collection title, title translations, and packed gallery items and returns
// a beautiful virtual gallery wall HTML string.
func GenerateHTML(title string, titleTranslations map[string]string, items []GalleryItem, wallAspect float64, isFull bool) (string, error) {
	tmplStr := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <!-- Attempt to load the app's current locale dynamically exported by the UI -->
    <script src="current_locale.js"></script>
    <style>
        body {
            background-color: #4a3b2c;
            margin: 0;
            padding: 60px 40px;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            align-items: center;
            font-family: system-ui, -apple-system, sans-serif;
            overflow-x: hidden;
        }

        h1 {
            color: rgba(255, 255, 255, 0.9);
            font-weight: 300;
            letter-spacing: 2px;
            margin-bottom: 60px;
            text-shadow: 0 4px 10px rgba(0,0,0,0.5);
            text-align: center;
            z-index: 10;
        }

        .gallery-wall {
            position: relative;
            width: 100%;
            max-width: 1100px;
            /* Matches the physical bounding box to prevent stretching percentages */
            aspect-ratio: {{.WallAspect}};
        }

        .artwork-container {
            position: absolute;
            transition: transform 0.3s ease;
        }

        .artwork-container:hover {
            transform: scale(1.03) translateY(-5px);
            z-index: 100;
        }

        .artwork {
            width: 100%;
            height: 100%;
            object-fit: contain;
            display: block;
        }

        .paper-label {
            position: absolute;
            background-color: #fdfbf7;
            border: 1px solid #dcd3c6;
            box-shadow: 2px 2px 5px rgba(0,0,0,0.3);
            color: #2c2c2c;
            padding: 10px 15px;
            font-family: "Georgia", serif;
            font-size: 13px;
            width: max-content;
            max-width: 200px;
            z-index: 1000;
            opacity: 0;
            pointer-events: none;
            transition: opacity 0.3s ease;
            display: flex;
            flex-direction: column;
            gap: 4px;
        }

        .paper-label .title { font-weight: bold; font-style: italic; }
        .paper-label .artist { font-weight: normal; }
        .paper-label .year { font-family: system-ui, -apple-system, sans-serif; font-size: 11px; color: #555; }

        .gallery-footer {
            width: 100%;
            max-width: 1100px;
            margin-top: 80px;
            margin-bottom: 20px;
            padding-top: 30px;
            border-top: 1px solid rgba(255, 255, 255, 0.1);
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 12px;
            color: rgba(255, 255, 255, 0.4);
            font-size: 14px;
            text-align: center;
            letter-spacing: 0.5px;
        }

        .gallery-footer a {
            color: rgba(255, 255, 255, 0.6);
            text-decoration: none;
            transition: color 0.2s ease;
        }

        .gallery-footer a:hover {
            color: rgba(255, 255, 255, 0.9);
        }

        .brand-link {
            font-weight: 600;
            letter-spacing: 1px;
            color: rgba(255, 255, 255, 0.7) !important;
        }

        .footer-links {
            display: flex;
            gap: 15px;
            font-size: 13px;
        }
    </style>
</head>
<body>
    {{if .IsFull}}
    <div style="position: absolute; top: 20px; left: 20px;">
        <a href="../index.html" style="color: rgba(255,255,255,0.7); text-decoration: none; font-size: 14px; letter-spacing: 0.5px; transition: color 0.2s ease;">
            <span style="margin-right: 5px;">&larr;</span> <span data-i18n="Back to Index">Back to Index</span>
        </a>
    </div>
    {{end}}
    <h1 data-i18n="{{.Title}}">{{.Title}}</h1>
    <div class="gallery-wall">
        {{range .Items}}
        <div class="artwork-container" style="left: {{.LeftPct}}%; top: {{.TopPct}}%; width: {{.WidthPct}}%; height: {{.HeightPct}}%;">
            {{if .ViewURL}}
            <a href="{{.ViewURL}}" target="_blank">
                <img class="artwork" src="{{.URL}}" alt="Artwork" />
            </a>
            {{else}}
            <img class="artwork" src="{{.URL}}" alt="Artwork" />
            {{end}}
            {{if or .Title .Artist}}
            <div class="paper-label">
                {{if .Title}}<div class="title">{{.Title}}</div>{{end}}
                {{if .Artist}}<div class="artist">{{.Artist}}</div>{{end}}
                {{if .Year}}<div class="year">{{.Year}}</div>{{end}}
            </div>
            {{end}}
        </div>
        {{end}}
    </div>
    
    <div class="gallery-footer">
        <div><span data-i18n="Curated by">Curated by</span> <a href="https://spicebox.dev/" target="_blank" class="brand-link">&#127798; Spice</a></div>
        <div class="footer-links">
            <a href="https://spicebox.dev/" target="_blank" data-i18n="Website">Website</a> &bull; 
            <a href="https://spicebox.dev/" target="_blank" data-i18n="Get for Windows">Get for Windows</a> &bull; 
            <a href="https://spicebox.dev/" target="_blank" data-i18n="Get for macOS">Get for macOS</a>
        </div>
        <div data-i18n="Artwork sourced from public domain collections." style="font-size: 12px; color: rgba(255, 255, 255, 0.3); margin-top: 5px;">
            Artwork sourced from public domain collections.
        </div>
    </div>
    <script>
        const translations = {{.TranslationsJSON}};
        const originalTitle = "{{.Title}}";

        document.addEventListener('DOMContentLoaded', () => {
            // Read locale from injected local script or fallback to browser
            let lang = window.spiceAppLocale;
            if (!lang) {
                lang = navigator.language.split('-')[0];
            }
            
            if (lang && translations[lang]) {
                const trans = translations[lang];
                document.querySelectorAll('[data-i18n]').forEach(el => {
                    const key = el.getAttribute('data-i18n');
                    if (trans[key]) el.textContent = trans[key];
                });
                
                // Update document title
                const tTitle = trans[originalTitle] || originalTitle;
                const tSpice = trans["Spice Gallery"] || "Spice Gallery";
                document.title = tTitle + " - " + tSpice;
            }

            // Paper label interactions
            const containers = document.querySelectorAll('.artwork-container');
            let hoverTimer;

            containers.forEach(container => {
                const label = container.querySelector('.paper-label');
                if (!label) return;

                container.addEventListener('mouseenter', (e) => {
                    hoverTimer = setTimeout(() => {
                        label.style.opacity = '1';
                    }, 1000);
                });
                
                container.addEventListener('mousemove', (e) => {
                    const rect = container.getBoundingClientRect();
                    const x = e.clientX - rect.left + 15;
                    const y = e.clientY - rect.top;
                    label.style.left = x + 'px';
                    label.style.top = y + 'px';
                });

                container.addEventListener('mouseleave', () => {
                    clearTimeout(hoverTimer);
                    label.style.opacity = '0';
                });
            });
        });
    </script>
</body>
</html>`

	t, err := template.New("gallery").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse gallery template: %w", err)
	}

	// For HTML safety, we construct a new slice with trusted URLs
	type SafeItem struct {
		LeftPct   float64
		TopPct    float64
		WidthPct  float64
		HeightPct float64
		URL       template.URL
		ViewURL   template.URL
		Title     string
		Artist    string
		Year      string
	}

	var safeItems []SafeItem
	for _, it := range items {
		safeItems = append(safeItems, SafeItem{
			LeftPct:   it.LeftPct,
			TopPct:    it.TopPct,
			WidthPct:  it.WidthPct,
			HeightPct: it.HeightPct,
			URL:       template.URL(it.URL),     //nolint:gosec // Our base64 data URIs and provider URLs are trusted
			ViewURL:   template.URL(it.ViewURL), //nolint:gosec // Provider View URLs are trusted
			Title:     it.Title,
			Artist:    it.Artist,
			Year:      it.Year,
		})
	}

	// Build translation dictionary for the UI strings
	uiKeys := []string{
		"Back to Index",
		"Curated by",
		"Website",
		"Get for Windows",
		"Get for macOS",
		"Artwork sourced from public domain collections.",
		"Spice Gallery",
	}

	// Keep strings from being marked stale by the AST extractor
	_ = i18n.T("Back to Index")
	_ = i18n.T("Curated by")
	_ = i18n.T("Website")
	_ = i18n.T("Get for Windows")
	_ = i18n.T("Get for macOS")
	_ = i18n.T("Artwork sourced from public domain collections.")
	_ = i18n.T("Spice Gallery")

	uiTrans := i18n.GetTranslationsForKeys(uiKeys)

	// Inject title translations into the map
	for langCode, localizedTitle := range titleTranslations {
		// Normalize JSON locales to app locales for the web gallery
		switch langCode {
		case "zh-CN":
			langCode = "zh"
		case "zh-TW":
			langCode = "zh-Hant"
		}

		if uiTrans[langCode] == nil {
			uiTrans[langCode] = make(map[string]string)
		}
		uiTrans[langCode][title] = localizedTitle
	}

	transBytes, _ := json.Marshal(uiTrans)

	data := struct {
		Title            string
		WallAspect       float64
		IsFull           bool
		Items            []SafeItem
		TranslationsJSON template.JS
	}{
		Title:            title,
		WallAspect:       wallAspect,
		IsFull:           isFull,
		Items:            safeItems,
		TranslationsJSON: template.JS(string(transBytes)), //nolint:gosec // JSON marshal is safe and escapes HTML characters
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute gallery template: %w", err)
	}

	return buf.String(), nil
}
