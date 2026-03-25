package localfolder

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

//go:embed localfolder.png
var iconData []byte

const ProviderName = "LocalFolder"

type Provider struct {
	cfg     *wallpaper.Config
	apiHost string
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg)
	})
}

func NewProvider(cfg *wallpaper.Config) *Provider {
	return &Provider{
		cfg:     cfg,
		apiHost: "127.0.0.1:49452",
	}
}

// SetTestConfig allows tests to override internal settings.
func (p *Provider) SetTestConfig(host string) {
	p.apiHost = host
}

func (p *Provider) ID() string {
	return wallpaper.LocalFolderProviderID
}

func (p *Provider) Name() string {
	return i18n.T("Local Folders")
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeLocal
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionIn
}

func (p *Provider) SupportsUserQueries() bool {
	return true
}

func (p *Provider) Title() string {
	return "Local Folders"
}

func (p *Provider) HomeURL() string {
	return ""
}

func (p *Provider) ParseURL(webURL string) (string, error) {
	// For Local Folder, the "URL" is actually an absolute folder path
	info, err := os.Stat(webURL)
	if err != nil {
		return "", fmt.Errorf("invalid folder path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", webURL)
	}
	return webURL, nil
}

func (p *Provider) EnrichImage(_ context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

// hasLocalImages checks if the given directory contains at least one image file.
// It uses os.Open and Readdir to stop searching as soon as it finds a single valid image,
// making it much faster for large directories.
func hasLocalImages(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()

	for {
		entries, err := f.Readdir(100) // Process in small batches
		if err != nil {
			break
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
				return true
			}
		}
	}
	return false
}

func (p *Provider) FetchImages(ctx context.Context, folderPath string, page int) ([]provider.Image, error) {
	// Short-circuit if folder is empty
	if !hasLocalImages(folderPath) {
		return []provider.Image{}, nil
	}

	collectionID := wallpaper.HashFolderPath(folderPath)
	host := p.apiHost
	u := fmt.Sprintf("http://%s/local/%s/%s/images?page=%d", host, wallpaper.LocalFolderNamespace, collectionID, page)

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
			ID:            fmt.Sprintf("LocalFolder_%s_%s", collectionID, d.ID),
			Path:          d.URL,
			Attribution:   d.Attribution,
			ViewURL:       d.ProductURL,
			Provider:      ProviderName,
			SourceQueryID: wallpaper.GenerateQueryID(wallpaper.LocalFolderProviderID + ":" + folderPath),
		}
	}
	return images, nil
}

func (p *Provider) CreateSettingsPanel(_ setting.SettingsManager) fyne.CanvasObject {
	vbox := container.NewVBox(
		widget.NewLabelWithStyle(i18n.T("Local Folders"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel(i18n.T("Browse to a folder on your computer containing wallpaper images.")),
	)

	if runtime.GOOS == "windows" {
		winHelp := widget.NewLabelWithStyle(
			i18n.T("Note (Windows): Due to OS limitations, to select a folder you must click on any image file inside the desired folder and then click 'Open'. The entire folder containing that image will be added."),
			fyne.TextAlignLeading,
			fyne.TextStyle{Italic: true},
		)
		winHelp.Wrapping = fyne.TextWrapWord
		vbox.Add(winHelp)
	}

	return vbox
}

func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) fyne.CanvasObject {
	imgQueryList := p.createImgQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	// Add Folder button
	addBtn := widget.NewButtonWithIcon(i18n.T("Add Folder"), theme.FolderOpenIcon(), func() {
		log.Debugf("[LocalFolder] Add Folder button clicked, opening OS picker...")
		showOSFolderPicker(sm.GetSettingsWindow(), func(folderPath string, err error) {
			log.Debugf("[LocalFolder] Picker callback: folderPath=%q, err=%v", folderPath, err)
			if err != nil {
				log.Debugf("[LocalFolder] Picker returned error: %v", err)
				return
			}
			if folderPath == "" {
				log.Debugf("[LocalFolder] Picker returned empty path (user cancelled)")
				return
			}

			// Wrap ALL UI manipulations in fyne.Do() to prevent deadlocking the app.
			fyne.Do(func() {
				// Show processing dialog immediately
				progressDialog := dialog.NewCustom(
					i18n.T("Processing Folder"),
					i18n.T("Please Wait..."),
					widget.NewProgressBarInfinite(),
					sm.GetSettingsWindow(),
				)
				progressDialog.Show()

				// Launch background scan
				go func() {
					// Guaranteed UI minimum duration so Fyne has time to actually paint the spinner
					// before we instantly hide it again (~300ms visual confirmation)
					time.Sleep(300 * time.Millisecond)

					// Use the full path as the description for better visibility
					desc := folderPath
					if len(desc) > 100 {
						desc = desc[:100]
					}

					log.Debugf("[LocalFolder] Adding local folder query: path=%q", folderPath)
					_, addErr := p.cfg.AddLocalFolderQuery(desc, folderPath, true)

					// Return to main thread to hide dialog and refresh UI
					fyne.Do(func() {
						progressDialog.Hide()

						if addErr != nil {
							log.Debugf("[LocalFolder] ERROR adding folder query: %v", addErr)
							dialog.ShowError(addErr, sm.GetSettingsWindow())
						} else {
							log.Debugf("[LocalFolder] Successfully added folder query")
							// Mark global settings as changed so Apply button enables
							sm.SetRefreshFlag("localfolder_add_" + folderPath)
							sm.GetCheckAndEnableApplyFunc()()
						}

						// Refresh the UI IMMEDIATELY
						imgQueryList.Refresh()
					})
				}()
			})
		})
	})

	return container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle(i18n.T("Local Folder Sources:"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabel(i18n.T("Add paths to folders on your computer containing wallpaper images.")),
			addBtn,
		),
		nil, nil, nil,
		imgQueryList,
	)
}

func (p *Provider) createImgQueryList(sm setting.SettingsManager) *widget.List {
	return wallpaper.CreateQueryList(sm, wallpaper.QueryListConfig{
		GetQueries:   p.cfg.GetLocalFolderQueries,
		EnableQuery:  p.cfg.EnableImageQuery,
		DisableQuery: p.cfg.DisableImageQuery,
		RemoveQuery:  p.cfg.RemoveLocalFolderQuery,
		GetDisplayText: func(q wallpaper.ImageQuery) string {
			return q.URL // Show full path as the link text
		},
		GetDisplayURL: func(q wallpaper.ImageQuery) *url.URL {
			u := storage.NewFileURI(q.URL)
			if res, err := url.Parse(u.String()); err == nil {
				return res
			}
			return nil
		},
	})
}

// ResolveNamespace is the DynamicNamespaceResolver callback for the API server.
// It maps collectionIDs (path hashes) to the actual folder paths from config.
func (p *Provider) ResolveNamespace(namespace, collectionID string) (string, bool) {
	if namespace != wallpaper.LocalFolderNamespace {
		return "", false
	}
	for _, q := range p.cfg.GetLocalFolderQueries() {
		if wallpaper.HashFolderPath(q.URL) == collectionID {
			return q.URL, true
		}
	}
	return "", false
}

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *Provider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource(ProviderName, iconData)
}
