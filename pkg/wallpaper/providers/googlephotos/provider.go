package googlephotos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
	"github.com/google/uuid"
)

// Provider implements ImageProvider for Google Photos via Picker & Download.
type Provider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
	auth       *Authenticator

	apiHost string
	rootDir string

	// Callback to update the query panel when auth state changes
	onAuthStatusChanged func()
}

func init() {
	wallpaper.RegisterProvider("GooglePhotos", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new Google Photos Provider.
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	return &Provider{
		cfg:        cfg,
		httpClient: client,
		auth:       NewAuthenticator(cfg, client),
		apiHost:    "127.0.0.1:49452",
		rootDir:    filepath.Join(os.TempDir(), "spice", "google_photos"),
	}
}

// SetTestConfig allows tests to override internal paths and hosts
func (p *Provider) SetTestConfig(host, rootDir string) {
	p.apiHost = host
	p.rootDir = rootDir
}

func (p *Provider) Name() string {
	return "GooglePhotos"
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeOnline
}

func (p *Provider) SupportsUserQueries() bool {
	return false
}

func (p *Provider) Title() string {
	return "Google Photos"
}

func (p *Provider) HomeURL() string {
	return "https://photos.google.com"
}

//go:embed GooglePhotos.png
var iconData []byte

func (p *Provider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("GooglePhotos", iconData)
}

// ParseURL handles internal Google Photos URLs.
// Scheme: googlephotos://<GUID>
func (p *Provider) ParseURL(webURL string) (string, error) {
	u, err := url.Parse(webURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "googlephotos" {
		return "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}
	return webURL, nil
}

func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	// Image path is already local or needs converting?
	// If path is absolute local path, it works.
	return img, nil
}

// FetchImages queries the local loopback API for images.
func (p *Provider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	// Parse URL to get GUID
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}
	guid := u.Host // googlephotos://<GUID> -> Host is GUID

	// Call Local API
	// Endpoint: /local/google_photos/{guid}/images?page={page}
	// Dynamic port for testing
	reqURL := fmt.Sprintf("http://%s/local/google_photos/%s/images?page=%d&per_page=24", p.apiHost, guid, page)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local api error: %d", resp.StatusCode)
	}

	var items []struct {
		ID          string `json:"id"`
		URL         string `json:"url"`
		Attribution string `json:"attribution"`
		ProductURL  string `json:"product_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	var images []provider.Image
	for _, item := range items {
		viewURL := item.ProductURL
		if viewURL == "" {
			viewURL = item.URL
		}
		images = append(images, provider.Image{
			ID:          item.ID,
			Path:        item.URL,
			ViewURL:     viewURL,
			Attribution: item.Attribution,
			Provider:    "GooglePhotos",
		})
	}

	return images, nil
}

func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	statusLabel := widget.NewLabel("Status: Checking...")

	var connectBtn *widget.Button
	var updateUI func()

	updateUI = func() {
		// Google Photos Picker requires Auth merely to launch the session.
		// We can keep the "Connected" state logic for visual reassurance,
		// but the core action is "Select Photos".
		token := p.cfg.GetGooglePhotosToken()
		if token != "" {
			expiry := p.cfg.GetGooglePhotosTokenExpiry()
			statusMsg := "Status: Authorized (Ready to Select)"
			if time.Now().After(expiry) {
				statusMsg += " (Token Expired)"
			}
			statusLabel.SetText(statusMsg)
			connectBtn.SetText("Disconnect Authorisation")
			connectBtn.OnTapped = func() {
				// revoke
				p.cfg.SetGooglePhotosToken("")
				updateUI()
			}
		} else {
			statusLabel.SetText("Status: Not Authorized")
			connectBtn.SetText("Authorize Google Photos")
			connectBtn.OnTapped = func() {
				err := p.auth.StartOAuthFlow(func(u *url.URL) error {
					return p.OpenBrowser(u.String())
				})
				if err != nil {
					dialog.ShowError(err, sm.GetSettingsWindow())
				} else {
					dialog.ShowInformation("Success", "Authorized!", sm.GetSettingsWindow())
					updateUI()
				}
			}
		}

		// Trigger cross-panel update
		if p.onAuthStatusChanged != nil {
			p.onAuthStatusChanged()
		}
	}

	connectBtn = widget.NewButton("Authorize", nil)
	updateUI()

	return container.NewVBox(
		widget.NewLabelWithStyle("Google Photos Integration", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		statusLabel,
		connectBtn,
	)
}

func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {

	imgQueryList := p.createImgQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	info := widget.NewLabel("Create New Wallpaper Collection:")

	progressBar := widget.NewProgressBarInfinite()
	progressBar.Hide()

	statusLabel := widget.NewLabel("")

	addBtn := widget.NewButton("Select Photos via Google Picker", nil)
	cancelBtn := widget.NewButton("Cancel", nil)
	cancelBtn.Importance = widget.LowImportance // Subtle
	cancelBtn.Hide()

	var cancelFunc context.CancelFunc

	// Logic to update button state based on auth
	updateBtnState := func() {
		token := p.cfg.GetGooglePhotosToken()
		if token != "" {
			addBtn.Enable()
			statusLabel.SetText("") // clear any "Please Authorize" message
		} else {
			addBtn.Disable()
			statusLabel.SetText("Please Authorize above first.")
		}
	}

	// Register callback
	p.onAuthStatusChanged = func() {
		// Run on UI thread to be safe, though callbacks usually are
		updateBtnState()
	}

	// Set initial state
	updateBtnState()

	// Cancel Action
	cancelBtn.OnTapped = func() {
		if cancelFunc != nil {
			cancelFunc() // Stop the background process
			statusLabel.SetText("Operation cancelled.")
		}
		cancelBtn.Hide()
		progressBar.Hide()
		addBtn.Show()
		addBtn.Enable()
	}

	addBtn.OnTapped = func() {
		if p.cfg.GetGooglePhotosToken() == "" {
			dialog.ShowError(fmt.Errorf("please authorize first"), sm.GetSettingsWindow())
			return
		}

		addBtn.Disable()
		addBtn.Hide()    // Hide "Select" to avoid confusion
		cancelBtn.Show() // Show "Cancel"

		statusLabel.SetText("Creating Picker Session...")
		progressBar.Show()

		// Create Cancellable Context
		ctx, cancel := context.WithCancel(context.Background())
		cancelFunc = cancel

		go func() {
			defer func() {
				// Cleanup context on exit if not cancelled manually
				// But wait, if we defer cancel(), it's fine.
				// We just need to make sure UI reset happens.
			}()

			// 1. Create Session
			session, err := p.CreatePickerSession(ctx)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				} // Silent exit
				p.uiError(sm, "Session Error", err, addBtn, progressBar, statusLabel, cancelBtn)
				return
			}

			// 2. Open Browser
			fyne.Do(func() {
				statusLabel.SetText("Please select photos in your browser...")
			})
			if err := p.OpenBrowser(session.PickerURI); err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				p.uiError(sm, "Browser Error", err, addBtn, progressBar, statusLabel, cancelBtn)
				return
			}

			// 3. Poll
			fyne.Do(func() {
				statusLabel.SetText("Waiting for selection (check browser)...")
			})
			finalSession, err := p.PollSession(ctx, session.ID, session.PollingConfig.PollInterval)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				p.uiError(sm, "Polling Error (Timed out?)", err, addBtn, progressBar, statusLabel, cancelBtn)
				return
			}

			// 4. Get Items
			fyne.Do(func() {
				statusLabel.SetText("Retrieving items...")
			})
			items, err := p.GetSessionItems(ctx, finalSession.ID)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				p.uiError(sm, "Retrieval Error", err, addBtn, progressBar, statusLabel, cancelBtn)
				return
			}

			if len(items) == 0 {
				p.uiError(sm, "No Items", fmt.Errorf("no photos selected"), addBtn, progressBar, statusLabel, cancelBtn)
				return
			}

			// 5. Download
			fyne.Do(func() {
				statusLabel.SetText(fmt.Sprintf("Downloading %d items...", len(items)))
			})

			guid := uuid.New().String()
			storageBase := p.rootDir
			targetDir := filepath.Join(storageBase, guid)

			// Download and get file links
			urlMap, err := p.DownloadItems(ctx, items, targetDir)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				p.uiError(sm, "Download Error", err, addBtn, progressBar, statusLabel, cancelBtn)
				return
			}

			// Pre-save metadata with links
			if err := p.saveInitialMetadata(guid, urlMap); err != nil {
				log.Printf("Failed to save initial metadata: %v", err)
			}

			// 6. Spawn Add Dialog (Main Thread)
			fyne.Do(func() {
				p.openAddGooglePhotosDialog(sm, guid, len(items), imgQueryList)

				// Reset UI
				cancelBtn.Hide()
				addBtn.Show()
				addBtn.Enable()
				progressBar.Hide()
				statusLabel.SetText("")
				cancelFunc = nil
			})
		}()
	}

	return container.NewBorder(
		container.NewVBox(info, container.NewStack(addBtn, cancelBtn), progressBar, statusLabel, widget.NewSeparator(), widget.NewLabel("My Collections:")),
		nil, nil, nil,
		imgQueryList,
	)
}

func (p *Provider) uiError(sm setting.SettingsManager, title string, err error, addBtn *widget.Button, bar *widget.ProgressBarInfinite, label *widget.Label, cancelBtn *widget.Button) {
	log.Printf("%s: %v", title, err)
	fyne.Do(func() {
		// dialog.ShowError(fmt.Errorf("%s: %v", title, err), sm.GetSettingsWindow()) // Optional: Don't popup on every error? User feedback in label is often enough.
		// Actually, let's keep it for real errors.
		dialog.ShowError(fmt.Errorf("%s: %v", title, err), sm.GetSettingsWindow())

		cancelBtn.Hide()
		addBtn.Show()
		addBtn.Enable()
		bar.Hide()
		label.SetText("Error: " + err.Error())
	})
}

// updateMetadata updates the description in metadata.json while preserving other fields.
func (p *Provider) updateMetadata(guid, description string) error {
	storageBase := p.rootDir
	targetDir := filepath.Join(storageBase, guid)
	metaFile := filepath.Join(targetDir, "metadata.json")

	// Ensure dir exists
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return nil
	}

	data := make(map[string]interface{})

	// 1. Read existing
	if f, err := os.Open(metaFile); err == nil {
		_ = json.NewDecoder(f).Decode(&data)
		f.Close()
	}

	// 2. Update
	data["id"] = guid
	data["description"] = description
	data["id"] = guid
	data["description"] = description
	// Author omitted or empty
	data["author"] = ""

	// 3. Write
	f, err := os.Create(metaFile)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(data)
}

// saveInitialMetadata creates the metadata.json with file links.
func (p *Provider) saveInitialMetadata(guid string, fileLinks map[string]string) error {
	storageBase := p.rootDir
	targetDir := filepath.Join(storageBase, guid)
	metaFile := filepath.Join(targetDir, "metadata.json")

	// Ensure dir exists (it usually does after download)
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return err
	}

	data := map[string]interface{}{
		"id":     guid,
		"author": "",
		"files":  fileLinks,
	}

	f, err := os.Create(metaFile)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(data)
}

// openAddGooglePhotosDialog shows a dialog to confirm the new collection.
func (p *Provider) openAddGooglePhotosDialog(sm setting.SettingsManager, guid string, count int, list *widget.List) {
	urlStr := "googlephotos://" + guid
	defaultDesc := fmt.Sprintf("Collection %s (%d items)", time.Now().Format("Jan 02 15:04"), count)

	// UI Elements
	// URL (Disabled)
	urlEntry := widget.NewEntry()
	urlEntry.SetText(urlStr)
	urlEntry.Disable()

	// Description (Editable)
	descEntry := widget.NewEntry()
	descEntry.SetText(defaultDesc)
	descEntry.PlaceHolder = "Enter description..."

	// Active (Check)
	activeCheck := widget.NewCheck("Active", nil)
	activeCheck.SetChecked(true)

	// Custom Dialog Content
	// Layout: Label / Entry pairs
	form := container.NewVBox(
		widget.NewLabel("Internal ID:"),
		urlEntry,
		widget.NewLabel("Description:"),
		descEntry,
		activeCheck,
	)

	d := dialog.NewCustomConfirm(
		"Save Collection",
		"Save",
		"Cancel",
		form,
		func(save bool) {
			if save {
				// Save Logic
				_, err := p.cfg.AddGooglePhotosQuery(descEntry.Text, urlStr, activeCheck.Checked)
				if err != nil {
					dialog.ShowError(err, sm.GetSettingsWindow())
					return
				}
				// Update Metadata
				_ = p.updateMetadata(guid, descEntry.Text)

				// Refresh
				sm.SetRefreshFlag("queries")
				sm.GetCheckAndEnableApplyFunc()()
				list.Refresh()
			} else {
				// Cancel Logic: Delete folder
				p.cleanupDownload(guid)
			}
		},
		sm.GetSettingsWindow(),
	)

	// Resize dialog to be usable
	d.Resize(fyne.NewSize(400, 350))
	d.Show()
}

func (p *Provider) cleanupDownload(guid string) {
	path := filepath.Join(p.rootDir, guid)
	os.RemoveAll(path)
}

func (p *Provider) createImgQueryList(sm setting.SettingsManager) *widget.List {
	// We need to fetch the latest state from config
	// But List widget callback uses index.
	// We handle deletion by refreshing the list widget itself.

	pendingState := make(map[string]bool)
	var queryList *widget.List

	queryList = widget.NewList(
		func() int {
			return len(p.cfg.GetGooglePhotosQueries())
		},
		func() fyne.CanvasObject {
			// Row Layout:
			// [Label: Description] (Expand) [Check: Active] [Button: Delete]
			label := widget.NewLabel("Collection Description")
			label.Truncation = fyne.TextTruncateEllipsis

			activeCheck := widget.NewCheck("", nil)
			delBtn := widget.NewButton("Delete", nil)

			rightGroup := container.NewHBox(widget.NewLabel("Active"), activeCheck, delBtn)
			return container.NewBorder(nil, nil, nil, rightGroup, label)
		},
		func(i int, o fyne.CanvasObject) {
			queries := p.cfg.GetGooglePhotosQueries()
			if i >= len(queries) {
				return
			}
			q := queries[i]
			queryKey := q.ID

			// Unpack UI
			c := o.(*fyne.Container)
			// Border layout Objects: [0] is Content (Entry)?
			// Actually Fyne internals are tricky. Use recursion or type check.
			var label *widget.Label
			var activeCheck *widget.Check
			var delBtn *widget.Button

			// Inspect objects known structure
			// Border: Center is Objects[0] (if only 1 center), etc.
			// Better to iterate.
			for _, obj := range c.Objects {
				if l, ok := obj.(*widget.Label); ok {
					label = l
				}
				if con, ok := obj.(*fyne.Container); ok {
					// Right Group
					for _, subObj := range con.Objects {
						if chk, ok := subObj.(*widget.Check); ok {
							activeCheck = chk
						}
						if btn, ok := subObj.(*widget.Button); ok {
							delBtn = btn
						}
					}
				}
			}

			if label != nil {
				label.SetText(q.Description)
			}

			if activeCheck != nil && delBtn != nil {
				// Bind Active Status
				activeCheck.OnChanged = nil
				if val, ok := pendingState[queryKey]; ok {
					activeCheck.SetChecked(val)
				} else {
					activeCheck.SetChecked(q.Active)
				}

				activeCheck.OnChanged = func(b bool) {
					pendingState[queryKey] = b
					sm.SetSettingChangedCallback(queryKey, func() {
						var err error
						if b {
							err = p.cfg.EnableGooglePhotosQuery(q.ID)
						} else {
							err = p.cfg.DisableGooglePhotosQuery(q.ID)
						}
						if err != nil {
							log.Printf("Failed to update query status: %v", err)
						}
						delete(pendingState, queryKey)
					})
					sm.SetRefreshFlag(queryKey)
					sm.GetCheckAndEnableApplyFunc()()
				}

				// Bind Delete
				delBtn.OnTapped = func() {
					dialog.ShowConfirm("Delete Collection", "Delete this collection and all local files?", func(b bool) {
						if b {
							// 1. Remove files
							u, _ := url.Parse(q.URL) // URL stores URL scheme
							if u != nil && u.Host != "" {
								guid := u.Host
								p.cleanupDownload(guid)
							}

							// 2. Remove Config
							if err := p.cfg.RemoveGooglePhotosQuery(q.ID); err != nil {
								dialog.ShowError(err, sm.GetSettingsWindow())
								return
							}
							sm.SetRefreshFlag("queries")
							queryList.Refresh()
						}
					}, sm.GetSettingsWindow())
				}
			}
		},
	)
	return queryList
}

func (p *Provider) OpenBrowser(urlStr string) error {
	u, _ := url.Parse(urlStr)
	return fyne.CurrentApp().OpenURL(u)
}
