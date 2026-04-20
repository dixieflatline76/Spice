package localfolder

import (
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *Provider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource(ProviderName, iconData)
}

// CreateSettingsPanel satisfies the ImageProvider interface.
// It is intentionally left as a stub because the framework uses CreateSettingsSchema instead.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}

// CreateQueryPanel creates the image query management panel.
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
