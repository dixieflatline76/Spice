package favorites

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
)

const (
	ProviderName = "Favorites"
)

type favJobType int

const (
	jobAdd favJobType = iota
	jobRemove
)

type favJob struct {
	jobType favJobType
	img     provider.Image
}

type Provider struct {
	cfg     *wallpaper.Config
	apiHost string
	rootDir string

	mu      sync.RWMutex
	favMap  map[string]bool
	jobChan chan favJob
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg)
	})
}

func NewProvider(cfg *wallpaper.Config) *Provider {
	p := &Provider{
		cfg:     cfg,
		apiHost: "127.0.0.1:49452",
		rootDir: filepath.Join(os.TempDir(), "spice", wallpaper.FavoritesCollection),
		favMap:  make(map[string]bool),
		jobChan: make(chan favJob, 100),
	}
	p.loadInitialMetadata()
	go p.runWorker()
	return p
}

// SetTestConfig allows tests to override internal paths and hosts
func (p *Provider) SetTestConfig(host, rootDir string) {
	p.apiHost = host
	p.rootDir = rootDir
}

func (p *Provider) Name() string {
	return ProviderName
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeLocal
}

func (p *Provider) Title() string {
	return "Favorites"
}

func (p *Provider) HomeURL() string {
	absPath, err := filepath.Abs(p.rootDir)
	if err != nil {
		log.Printf("Failed to resolve favorites dir: %v", err)
		return ""
	}
	// Convert to URI-friendly path (forward slashes)
	// On Windows: C:/Users/... -> file:///C:/Users/...
	path := filepath.ToSlash(absPath)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + path
}

func (p *Provider) GetProviderIcon() fyne.Resource {
	res, err := p.cfg.GetAssetManager().GetIcon("favorite.png")
	if err != nil {
		log.Printf("Failed to load favorite.png: %v", err)
		return theme.SettingsIcon()
	}
	return res
}

func (p *Provider) ParseURL(webURL string) (string, error) {
	if webURL == wallpaper.FavoritesQueryID {
		return webURL, nil
	}
	return "", fmt.Errorf("invalid favorites URL")
}

func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

func (p *Provider) IsFavorited(img provider.Image) bool {
	if img.Provider == ProviderName {
		return true
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.favMap[img.ID]
}

func (p *Provider) loadInitialMetadata() {
	p.mu.Lock()
	defer p.mu.Unlock()

	favDir := p.rootDir
	if _, err := os.Stat(favDir); os.IsNotExist(err) {
		return
	}

	metaFile := filepath.Join(favDir, "metadata.json")
	var meta map[string]interface{}
	if f, err := os.ReadFile(metaFile); err == nil {
		if err := json.Unmarshal(f, &meta); err == nil {
			if filesMeta, ok := meta["files"].(map[string]interface{}); ok {
				for filename := range filesMeta {
					// ID is filename without extension
					id := strings.TrimSuffix(filename, filepath.Ext(filename))
					p.favMap[id] = true
				}
			}
		}
	}
}

func (p *Provider) runWorker() {
	for job := range p.jobChan {
		switch job.jobType {
		case jobAdd:
			p.addFavoriteInternal(job.img)
		case jobRemove:
			p.removeFavoriteInternal(job.img)
		}
	}
}

func (p *Provider) GetSourceQueryID() string {
	return wallpaper.FavoritesQueryID
}

func (p *Provider) AddFavorite(img provider.Image) error {
	p.mu.Lock()
	p.favMap[img.ID] = true
	p.mu.Unlock()

	p.jobChan <- favJob{jobType: jobAdd, img: img}
	return nil
}

func (p *Provider) addFavoriteInternal(img provider.Image) {
	favDir := p.rootDir
	if err := os.MkdirAll(favDir, 0755); err != nil {
		log.Printf("failed to create favorites directory: %v", err)
		return
	}

	filename := filepath.Base(img.FilePath)
	destPath := filepath.Join(favDir, filename)

	// FIFO Logic: Limit to 200 images
	entries, err := os.ReadDir(favDir)
	if err == nil {
		var images []os.DirEntry
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) != ".json" {
				images = append(images, e)
			}
		}
		if len(images) >= wallpaper.MaxFavoritesLimit {
			var oldest string
			var oldestTime time.Time
			first := true
			for _, entry := range images {
				if info, err := os.Stat(filepath.Join(favDir, entry.Name())); err == nil {
					if first || info.ModTime().Before(oldestTime) {
						oldestTime = info.ModTime()
						oldest = entry.Name()
						first = false
					}
				}
			}
			if oldest != "" {
				os.Remove(filepath.Join(favDir, oldest))
				// Also remove from favMap
				oldestID := strings.TrimSuffix(oldest, filepath.Ext(oldest))
				p.mu.Lock()
				delete(p.favMap, oldestID)
				p.mu.Unlock()
				log.Printf("FIFO: Removed oldest favorite %s", oldest)
			}
		}
	}

	// Perform Copy
	input, err := os.ReadFile(img.FilePath)
	if err != nil {
		log.Printf("failed to read master for favoriting: %v", err)
		return
	}
	if err := os.WriteFile(destPath, input, 0600); err != nil {
		log.Printf("failed to save favorite: %v", err)
		return
	}

	// Save Metadata
	metaFile := filepath.Join(favDir, "metadata.json")
	var meta map[string]interface{}
	if f, err := os.ReadFile(metaFile); err == nil {
		if err := json.Unmarshal(f, &meta); err != nil {
			log.Printf("Failed to unmarshal favorites metadata: %v", err)
		}
	}
	if meta == nil {
		meta = make(map[string]interface{})
	}
	filesMeta, ok := meta["files"].(map[string]interface{})
	if !ok {
		filesMeta = make(map[string]interface{})
	}
	filesMeta[filename] = map[string]string{
		"attribution": img.Attribution,
		"product_url": img.ViewURL,
	}
	meta["files"] = filesMeta
	if data, err := json.MarshalIndent(meta, "", "  "); err == nil {
		if err := os.WriteFile(metaFile, data, 0600); err != nil {
			log.Printf("Failed to save favorites metadata: %v", err)
		}
	}
}

func (p *Provider) RemoveFavorite(img provider.Image) error {
	p.mu.Lock()
	delete(p.favMap, img.ID)
	p.mu.Unlock()

	p.jobChan <- favJob{jobType: jobRemove, img: img}
	return nil
}

func (p *Provider) removeFavoriteInternal(img provider.Image) {
	favDir := p.rootDir

	// We cannot rely on filepath.Base(img.FilePath) because img.FilePath might point to a
	// processed/cached copy (e.g. .jpeg) while the original favorite is .png or .jpg.
	// We use img.ID to find the file.

	// Strategy:
	// 1. Check metadata (authoritative source of filenames)
	// 2. Glob search (fallback)

	var filename string
	metaFile := filepath.Join(favDir, "metadata.json")
	var meta map[string]interface{}

	if f, err := os.ReadFile(metaFile); err == nil {
		if err := json.Unmarshal(f, &meta); err == nil {
			if filesMeta, ok := meta["files"].(map[string]interface{}); ok {
				// Search for filename matching ID
				for k := range filesMeta {
					if strings.TrimSuffix(k, filepath.Ext(k)) == img.ID {
						filename = k
						break
					}
				}
			}
		}
	}

	// Fallback if not found in metadata (or metadata missing)
	if filename == "" {
		matches, err := filepath.Glob(filepath.Join(favDir, img.ID+".*"))
		if err == nil && len(matches) > 0 {
			filename = filepath.Base(matches[0])
		}
	}

	if filename == "" {
		// Last resort: use the cached filename (likely wrong extension, but worth a try)
		filename = filepath.Base(img.FilePath)
	}

	destPath := filepath.Join(favDir, filename)
	if err := os.Remove(destPath); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("failed to remove favorite file %s: %v", destPath, err)
		}
	}

	// Cleanup Metadata Entry
	if meta != nil {
		if filesMeta, ok := meta["files"].(map[string]interface{}); ok {
			if _, exists := filesMeta[filename]; exists {
				delete(filesMeta, filename)
				if data, err := json.MarshalIndent(meta, "", "  "); err == nil {
					if err := os.WriteFile(metaFile, data, 0600); err != nil {
						log.Printf("Failed to update favorites metadata: %v", err)
					}
				}
			}
		}
	}
}

func (p *Provider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	// Call Local API: /local/favorites/default/images
	// The actual URL built by the system will be local loopback

	// Optimization: Short-circuit if local folder is empty (or only has metadata)
	// This prevents "micro pauses" from unnecessary HTTP requests to the local server
	entries, err := os.ReadDir(p.rootDir)
	if err == nil {
		hasImages := false
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if name == "metadata.json" || name == ".ds_store" {
				continue
			}
			// Check for common image extensions
			if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") ||
				strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".webp") {
				hasImages = true
				break
			}
		}
		if !hasImages {
			return []provider.Image{}, nil
		}
	}

	// Construct the local API URL
	host := p.apiHost
	u := fmt.Sprintf("http://%s/local/%s/%s/images?page=%d", host, wallpaper.FavoritesNamespace, wallpaper.FavoritesCollection, page)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local API returned status: %d", resp.StatusCode)
	}

	var respData []struct {
		ID          string `json:"id"`
		URL         string `json:"url"`
		Attribution string `json:"attribution"`
		ProductURL  string `json:"product_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, err
	}

	images := make([]provider.Image, len(respData))
	for i, d := range respData {
		images[i] = provider.Image{
			ID:          d.ID,
			Path:        d.URL, // Map local API 'url' to 'Path' (download)
			Attribution: d.Attribution,
			ViewURL:     d.ProductURL, // Map local API 'product_url' to 'ViewURL'
			Provider:    ProviderName,
		}
	}
	return images, nil
}

func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	clearBtn := widget.NewButtonWithIcon("Clear All Favorites", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("Clear All Favorites", "Are you sure you want to delete all saved favorites?", func(b bool) {
			if b {
				path := p.rootDir
				os.RemoveAll(path)
				if err := os.MkdirAll(path, 0755); err != nil {
					log.Printf("Failed to create favorites directory: %v", err)
				}
				log.Println("Favorites cleared.")
				// Logic to refresh plugin will be triggered by Apply if we mark it?
				// Actually this is immediate.
			}
		}, sm.GetSettingsWindow())
	})
	clearBtn.Importance = widget.DangerImportance

	return container.NewVBox(
		widget.NewLabelWithStyle("Favorites Management", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Wipe all local favorites from your temp folder."),
		clearBtn,
	)
}

func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	// Single row: "Favorite Images" with Active toggle

	// Check if query exists, if not create it (auto-activation usually happens on first favorite)
	// But for UI display we should ensure it's "visible" or at least represented.

	query, exists := p.cfg.GetQuery(wallpaper.FavoritesQueryID)

	label := widget.NewLabel("Favorite Images")
	activeCheck := widget.NewCheck("Active", nil)
	activeCheck.SetChecked(query.Active)

	activeCheck.OnChanged = func(b bool) {
		sm.SetSettingChangedCallback(wallpaper.FavoritesQueryID, func() {
			var err error
			if b {
				// Ensure it exists in config
				if !exists {
					_, err = p.cfg.AddFavoritesQuery("Favorite Images", wallpaper.FavoritesQueryID, true)
				} else {
					err = p.cfg.EnableImageQuery(wallpaper.FavoritesQueryID)
				}
			} else {
				err = p.cfg.DisableImageQuery(wallpaper.FavoritesQueryID)
			}
			if err != nil {
				log.Printf("Failed to toggle favorites: %v", err)
			}
			sm.RebuildTrayMenu()
		})
		sm.SetRefreshFlag(wallpaper.FavoritesQueryID)
		sm.GetCheckAndEnableApplyFunc()()
	}

	return container.NewVBox(
		widget.NewLabel("Wallpaper Sources:"),
		container.NewHBox(label, layout.NewSpacer(), activeCheck),
	)
}
