package favorites

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

type Provider struct {
	cfg *wallpaper.Config
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg)
	})
}

func NewProvider(cfg *wallpaper.Config) *Provider {
	return &Provider{cfg: cfg}
}

func (p *Provider) Name() string {
	return ProviderName
}

func (p *Provider) Title() string {
	return "Favorites"
}

func (p *Provider) HomeURL() string {
	return ""
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
	// If the image is served from the Favorites provider itself, it's definitely favorited.
	if img.Provider == ProviderName {
		return true
	}

	if img.FilePath == "" {
		return false
	}
	favDir := filepath.Join(os.TempDir(), "spice", wallpaper.FavoritesCollection)
	destPath := filepath.Join(favDir, filepath.Base(img.FilePath))
	_, err := os.Stat(destPath)
	return err == nil
}

func (p *Provider) GetSourceQueryID() string {
	return wallpaper.FavoritesQueryID
}

func (p *Provider) AddFavorite(img provider.Image) error {
	favDir := filepath.Join(os.TempDir(), "spice", wallpaper.FavoritesCollection)
	if err := os.MkdirAll(favDir, 0755); err != nil {
		return fmt.Errorf("failed to create favorites directory: %w", err)
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
				log.Printf("FIFO: Removed oldest favorite %s", oldest)
			}
		}
	}

	// Perform Copy
	input, err := os.ReadFile(img.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read master for favoriting: %w", err)
	}
	if err := os.WriteFile(destPath, input, 0644); err != nil {
		return fmt.Errorf("failed to save favorite: %w", err)
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
		if err := os.WriteFile(metaFile, data, 0644); err != nil {
			log.Printf("Failed to save favorites metadata: %v", err)
		}
	}

	return nil
}

func (p *Provider) RemoveFavorite(img provider.Image) error {
	favDir := filepath.Join(os.TempDir(), "spice", wallpaper.FavoritesCollection)
	filename := filepath.Base(img.FilePath)
	destPath := filepath.Join(favDir, filename)

	if err := os.Remove(destPath); err != nil {
		return fmt.Errorf("failed to remove favorite file: %w", err)
	}

	// Cleanup Metadata
	metaFile := filepath.Join(favDir, "metadata.json")
	if f, err := os.ReadFile(metaFile); err == nil {
		var meta map[string]interface{}
		if err := json.Unmarshal(f, &meta); err == nil {
			if filesMeta, ok := meta["files"].(map[string]interface{}); ok {
				delete(filesMeta, filename)
				if data, err := json.MarshalIndent(meta, "", "  "); err == nil {
					if err := os.WriteFile(metaFile, data, 0644); err != nil {
						log.Printf("Failed to update favorites metadata: %v", err)
					}
				}
			}
		}
	}
	return nil
}

func (p *Provider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	// Call Local API: /local/favorites/default/images
	// The actual URL built by the system will be local loopback

	// Construct the local API URL
	host := "127.0.0.1:49452" // Standard Spice API port
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
				path := filepath.Join(os.TempDir(), "spice", wallpaper.FavoritesCollection)
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
	if !exists {
		// Placeholder for UI if not yet "favorited" anything?
		// User said: "be automatically added or visible once a user favorites their first image, or simply have it always available as a source"
		// Let's make it always available in the UI.
		query = wallpaper.ImageQuery{
			ID:          wallpaper.FavoritesQueryID,
			Description: "Favorite Images",
			URL:         wallpaper.FavoritesQueryID,
			Active:      false,
			Provider:    ProviderName,
		}
	}

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
