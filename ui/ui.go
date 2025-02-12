package ui

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/util"

	"github.com/dixieflatline76/Spice/service"
)

// SpiceApp represents the application
type SpiceApp struct {
	app      fyne.App
	assetMgr *asset.Manager
	trayMenu *fyne.Menu
	prefs    fyne.Preferences
	cfg      *config.Config
}

var (
	instance *SpiceApp // Singleton instance of the application
	once     sync.Once // Ensures the singleton is created only once
)

// GetInstance returns the singleton instance of the application
func GetInstance() *SpiceApp {
	// Create a new instance of the application if it doesn't exist
	a := app.NewWithID(config.ServiceName)
	c := config.GetConfig(a.Preferences())
	if _, ok := a.(desktop.App); ok {
		once.Do(func() {
			instance = &SpiceApp{
				app:      a,
				assetMgr: asset.NewManager(),
				prefs:    a.Preferences(),
				cfg:      c,
			}
			instance.CreateTrayMenu()
			instance.verifyEULA()
		})
		return instance
	}
	log.Println("Tray icon not supported on this platform")
	return nil
}

// CreateTrayMenu creates the tray menu for the application
func (sa *SpiceApp) CreateTrayMenu() {
	desk := sa.app.(desktop.App)
	trayIcon, _ := sa.assetMgr.GetIcon("tray.png")
	trayMenu := fyne.NewMenu(
		config.ServiceName,
		sa.createMenuItem("Next Wallpaper", func() {
			go service.SetNextWallpaper()
		}, "next.png"),
		sa.createMenuItem("Prev Wallpaper", func() {
			go service.SetPreviousWallpaper()
		}, "prev.png"),
		sa.createMenuItem("Pick for Me", func() {
			go service.SetRandomWallpaper()
		}, "rand.png"),
		fyne.NewMenuItemSeparator(), // Divider line
		sa.createMenuItem("Image Page", func() {
			go service.ViewCurrentImageOnWeb(sa.app)
		}, "view.png"),
		fyne.NewMenuItemSeparator(), // Divider line
		sa.createMenuItem("Preferences", func() {
			go sa.CreatePreferencesWindow()
		}, "prefs.png"),
		sa.createMenuItem("About Spice", func() {
			go sa.CreateSplashScreen()
		}, "tray.png"),
		fyne.NewMenuItemSeparator(), // Divider line
		sa.createMenuItem("Quit", func() {
			// Stop the service before quitting the application
			sa.app.Quit()
		}, "quit.png"),
	)
	desk.SetSystemTrayMenu(trayMenu)
	desk.SetSystemTrayIcon(trayIcon)
	sa.app.SetIcon(trayIcon)
	sa.trayMenu = trayMenu
}

func (sa *SpiceApp) createMenuItem(label string, action func(), iconName string) *fyne.MenuItem {
	mi := fyne.NewMenuItem(label, action)
	icon, err := sa.assetMgr.GetIcon(iconName)
	if err != nil {
		log.Printf("Failed to load icon: %v", err)
		return mi
	}
	mi.Icon = icon
	return mi
}

// addVersionWatermark adds a version watermark to the given image.
func (sa *SpiceApp) addVersionWatermark(img image.Image) (image.Image, error) {

	versionString := fmt.Sprintf("Version: %s", config.AppVersion)

	// Create a watermark image
	watermark := imaging.New(img.Bounds().Dx(), img.Bounds().Dy(), color.Transparent)

	// Add the label directly to the watermark image
	col := color.RGBA{100, 50, 0, 200} // Dark brown with some transparency

	// Calculate the width of the text
	bounds, _ := font.BoundString(basicfont.Face7x13, versionString)
	textWidth := bounds.Max.X.Ceil()

	point := fixed.Point26_6{
		X: fixed.Int26_6((img.Bounds().Dx() - textWidth - 10) * 64), // Offset from right edge, accounting for text width
		Y: fixed.Int26_6((img.Bounds().Dy() - 10) * 64),             // Offset from bottom edge
	}

	d := &font.Drawer{
		Dst:  watermark,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13, // Use the basic font
		Dot:  point,
	}
	d.DrawString(versionString)

	// Overlay the watermark
	dst := imaging.Overlay(img, watermark, image.Pt(0, 0), 1)
	return dst, nil
}

// CreateSplashScreen creates a splash screen for the application
func (sa *SpiceApp) CreateSplashScreen() {
	// Create a splash screen with the application icon
	drv, ok := sa.app.Driver().(desktop.Driver)
	if !ok {
		log.Println("Splash screen not supported")
		return // Splash screen not supported
	}

	splashWindow := drv.CreateSplashWindow()

	// Load the splash image
	splashImg, err := sa.assetMgr.GetImage("splash.png")
	if err != nil {
		log.Fatalf("Failed to load splash image: %v", err)
	}

	// Create a watermark image
	watermarkSplashImg, err := sa.addVersionWatermark(splashImg)
	if err == nil {
		splashImg = watermarkSplashImg
	}

	// Create an image canvas object
	img := canvas.NewImageFromImage(splashImg)
	img.FillMode = canvas.ImageFillOriginal // Ensure the image keeps its original size

	// Set the splash window content and show it
	splashWindow.SetContent(img)
	splashWindow.Resize(fyne.NewSize(300, 300))
	splashWindow.CenterOnScreen()
	splashWindow.Show()

	// Hide the splash screen after 3 seconds
	go func() {
		time.Sleep(3 * time.Second)
		splashWindow.Close() // Close the splash window
	}()
}

// createSectionTitleLabel creates a label for a setting title
func createSectionTitleLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.HighImportance
	label.TextStyle = fyne.TextStyle{Bold: true}
	return label
}

// createSettingTitleLabel creates a label for a setting title
func createSettingTitleLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.MediumImportance
	label.TextStyle = fyne.TextStyle{Bold: true}
	return label
}

// createSettingDescriptionLabel creates a label for a setting description
func createSettingDescriptionLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.LowImportance
	label.TextStyle = fyne.TextStyle{Italic: true}
	return label
}

func (sa *SpiceApp) createImgQueryList(prefsWindow fyne.Window, refresh *bool, checkAndEnableApply func()) *widget.List {

	var queryList *widget.List
	queryList = widget.NewList(
		func() int {
			return len(sa.cfg.ImageQueries)
		},
		func() fyne.CanvasObject {
			urlLink := widget.NewHyperlink("Placeholder", nil) // Placeholder text
			activeCheck := widget.NewCheck("Active", nil)
			deleteButton := widget.NewButton("Delete", nil)

			return container.NewHBox(urlLink, layout.NewSpacer(), activeCheck, deleteButton)
		},
		func(i int, o fyne.CanvasObject) {

			c := o.(*fyne.Container)

			urlLink := c.Objects[0].(*widget.Hyperlink)
			urlLink.SetText(sa.cfg.ImageQueries[i].Description)

			siteURL := service.GetWallhavenURL(sa.cfg.ImageQueries[i].URL)
			if siteURL != nil {
				urlLink.SetURL(siteURL)
			} else {
				// this should never happen
				// TODO refactor later
				urlLink.SetURLFromString(sa.cfg.ImageQueries[i].URL)
			}

			activeCheck := c.Objects[2].(*widget.Check)
			deleteButton := c.Objects[3].(*widget.Button)

			activeCheck.SetChecked(sa.cfg.ImageQueries[i].Active)
			activeCheck.OnChanged = func(b bool) {
				if b == sa.cfg.ImageQueries[i].Active {
					return // no change, just return
				}

				if b {
					sa.cfg.EnableImageQuery(i)
				} else {
					sa.cfg.DisableImageQuery(i)
				}

				*refresh = true
				checkAndEnableApply()
			}

			deleteButton.OnTapped = func() {
				d := dialog.NewConfirm("Please Confirm", fmt.Sprintf("Are you sure you want to delete %s?", sa.cfg.ImageQueries[i].Description), func(b bool) {
					if b {
						if sa.cfg.ImageQueries[i].Active {
							*refresh = true
							checkAndEnableApply()
						}

						sa.cfg.RemoveImageQuery(i)
						queryList.Refresh()
					}

				}, prefsWindow)
				d.Show()
			}
		},
	)
	return queryList
}

func (sa *SpiceApp) createImageQueryPanel(prefsWindow fyne.Window, parent *fyne.Container, refresh *bool, checkAndEnableApply func()) {

	imgQueryList := sa.createImgQueryList(prefsWindow, refresh, checkAndEnableApply)
	var addButton *widget.Button
	addButton = widget.NewButton("Add Image Query", func() {

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("Cut and paste your wallhaven image query URL here")

		descEntry := widget.NewEntry()
		descEntry.SetPlaceHolder("Add a description for this query")

		formStatus := widget.NewLabel("")
		activeBool := widget.NewCheck("Active", nil)

		cancelButton := widget.NewButton("Cancel", nil)
		actionButton := widget.NewButton("Save", nil)
		actionButton.Disable() // Save button is only enabled when the URL is valid and min desc has been added

		formValidator := func(who *widget.Entry) bool {
			urlStrErr := urlEntry.Validate()
			descStrErr := descEntry.Validate()

			if urlStrErr == nil && descStrErr == nil {
				formStatus.SetText("Everything looks good")
				formStatus.Importance = widget.SuccessImportance
				formStatus.Refresh()
				return true
			}

			if who == urlEntry {
				if urlStrErr != nil {
					formStatus.SetText(urlStrErr.Error())
					formStatus.Importance = widget.DangerImportance
				} else {
					formStatus.SetText("URL OK")
					formStatus.Importance = widget.SuccessImportance
				}
			}

			if who == descEntry {
				if descStrErr != nil {
					formStatus.SetText(descStrErr.Error())
					formStatus.Importance = widget.DangerImportance
				} else {
					formStatus.SetText("Description OK")
					formStatus.Importance = widget.SuccessImportance
				}
			}

			formStatus.Refresh()
			return false
		}

		urlEntry.Validator = validation.NewRegexp(service.WallhavenURLRegexp, "Invalid wallhaven image query URL pattern")
		descEntry.Validator = validation.NewRegexp(service.WallhavenDescRegexp, "Invalid description must be between 5 and 800 characters long")

		urlEntry.OnChanged = func(s string) {
			if formValidator(urlEntry) {
				actionButton.Enable()
			} else {
				actionButton.Disable()
			}
		}

		descEntry.OnChanged = func(s string) {
			if formValidator(descEntry) {
				actionButton.Enable()
			} else {
				actionButton.Disable()
			}
		}

		c := container.NewVBox()
		c.Add(createSettingTitleLabel("wallhaven Image Query URL:"))
		c.Add(urlEntry)
		c.Add(createSettingTitleLabel("Description:"))
		c.Add(descEntry)
		c.Add(formStatus)
		c.Add(widget.NewSeparator())
		c.Add(activeBool)
		c.Add(widget.NewSeparator())
		c.Add(container.NewHBox(cancelButton, layout.NewSpacer(), actionButton))

		d := dialog.NewCustomWithoutButtons("New Image Query", c, prefsWindow)
		d.Resize(fyne.NewSize(800, 200))

		actionButton.OnTapped = func() {

			apiURL := service.CovertToAPIURL(urlEntry.Text)
			err := service.CheckWallhavenURL(apiURL)
			if err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return
			}

			sa.cfg.AddImageQuery(descEntry.Text, apiURL, activeBool.Checked)

			addButton.Enable()
			imgQueryList.Refresh()

			if activeBool.Checked {
				*refresh = true
				checkAndEnableApply()
			}
			d.Hide()
			addButton.Enable()
		}

		cancelButton.OnTapped = func() {
			d.Hide()
			addButton.Enable()
		}

		d.Show()
		addButton.Disable()
		parent.Refresh()
	})

	header := container.NewVBox()
	header.Add(widget.NewSeparator())
	header.Add(createSettingTitleLabel("wallhaven Image Queries"))
	header.Add(createSettingDescriptionLabel("Manage your wallhaven.cc image queries here. Spice will convert query URL into wallhaven API format."))
	header.Add(addButton)
	qpContainer := container.NewBorder(header, nil, nil, nil, imgQueryList)
	parent.Add(qpContainer)
	parent.Refresh()
}

// CreateWallpaperPreferences creates a preferences widget for wallpaper settings
func (sa *SpiceApp) createWallpaperPreferences(prefsWindow fyne.Window) *fyne.Container {
	header := container.NewVBox()
	footer := container.NewVBox()

	prefsPanel := container.NewBorder(header, footer, nil, nil)

	header.Add(createSectionTitleLabel("Wallpaper Preferences"))
	header.Add(createSettingDescriptionLabel("Following settings control the general behavior of wallpaper changes across all image services."))

	var checkAndEnableApply func()  // Function to check if any setting has changed and enable/disable the apply button
	refresh, chgFrq := false, false // Flags to track if the wallpaper settings have changed

	// Change Frequency (using the enum)
	frequencyOptions := []string{}
	for _, f := range service.GetFrequencies() {
		frequencyOptions = append(frequencyOptions, f.String())
	}

	initialFrequencyInt := sa.prefs.IntWithFallback(service.WallpaperChgFreqPrefKey, int(service.FrequencyHourly)) // Default to hourly
	intialFrequency := service.Frequency(initialFrequencyInt)

	frequencySelect := widget.NewSelect(frequencyOptions, func(selected string) {})
	frequencySelect.SetSelectedIndex(initialFrequencyInt)

	header.Add(widget.NewSeparator())
	header.Add(createSettingTitleLabel("Change Frequency:"))
	header.Add(createSettingDescriptionLabel("Select how often you want your wallpaper to change."))
	header.Add(frequencySelect)

	// 3. Smart Fit
	initialSmartFit := sa.prefs.BoolWithFallback(service.SmartFitPrefKey, false)

	smartFitCheck := widget.NewCheck("Enable Smart Fit", func(b bool) {})
	smartFitCheck.SetChecked(initialSmartFit)

	header.Add(widget.NewSeparator())
	header.Add(createSettingTitleLabel("Smart Fit:"))
	header.Add(createSettingDescriptionLabel("Enable Smart Fit to automatically scale and crop the wallpaper to fit your screen resolution."))
	header.Add(smartFitCheck)

	//wallhaven service section
	header.Add(createSectionTitleLabel("wallhaven Service Preferences"))
	header.Add(createSettingDescriptionLabel("Following settings are only used for wallhaven.cc image service."))

	// wallhaven API Key
	wallhavenKeyEntry := widget.NewEntry()
	wallhavenKeyEntry.SetPlaceHolder("Enter your wallhaven.cc API Key")
	initialKey := sa.prefs.StringWithFallback(service.WallhavenAPIKeyPrefKey, "")
	wallhavenKeyEntry.SetText(initialKey)
	statusLabel := widget.NewLabel("")

	wallhavenKeyEntry.Validator = validation.NewRegexp(service.WallhavenAPIKeyRegexp, "wallhaven API keys are 32 alpha numerics characters")
	wallhavenKeyEntry.OnChanged = func(s string) {
		entryErr := wallhavenKeyEntry.Validate()
		if entryErr != nil {
			statusLabel.SetText(entryErr.Error())
			statusLabel.Importance = widget.DangerImportance
		} else {
			keyErr := service.CheckWallhavenAPIKey(s)
			if keyErr != nil {
				statusLabel.SetText(keyErr.Error())
				statusLabel.Importance = widget.DangerImportance
			} else {
				statusLabel.SetText("API Key OK")
				statusLabel.Importance = widget.SuccessImportance
				if initialKey != s {
					sa.prefs.SetString(service.WallhavenAPIKeyPrefKey, s) // Save immediately
					refresh = true
					checkAndEnableApply()
				}
			}
		}
		statusLabel.Refresh()
	}

	header.Add(widget.NewSeparator())
	header.Add(createSettingTitleLabel("wallhaven API Key:"))
	header.Add(createSettingDescriptionLabel("Enter your API Key from wallhaven.cc to enable wallpaper downloads from this source."))
	header.Add(wallhavenKeyEntry)
	header.Add(statusLabel)

	// Apply Button (Initially Disabled)
	var applyButton *widget.Button
	applyButton = widget.NewButton("Apply Changes", func() {

		// Disable the apply button immediate as RefreshImages is slow
		applyButton.Disable()

		// Refresh images if API Key has changed or smart fit has been toggled
		if refresh {
			service.RefreshImages()
			refresh = false
		}

		// Change wallpaper frequency
		if chgFrq {
			selectedFrequency := service.Frequency(frequencySelect.SelectedIndex())
			service.ChangeWallpaperFrequency(selectedFrequency.Duration())
			chgFrq = false
		}
	})
	applyButton.Disable() // Start as disabled

	// Function to check if any setting has changed and enable/disable the apply button
	checkAndEnableApply = func() {
		if refresh || chgFrq {
			applyButton.Enable()
		} else {
			applyButton.Disable()
		}
		applyButton.Refresh()
	}

	frequencySelect.OnChanged = func(s string) {
		for _, f := range service.GetFrequencies() {
			if f.String() == s && f != intialFrequency {
				sa.prefs.SetInt(service.WallpaperChgFreqPrefKey, int(f))
				intialFrequency = f
				// Log the selected frequency and duration
				chgFrq = true
				break
			}
		}
		checkAndEnableApply()
	}

	smartFitCheck.OnChanged = func(b bool) {
		if initialSmartFit != b {
			sa.prefs.SetBool(service.SmartFitPrefKey, b)
			service.SetSmartFitEnabled(b)
			refresh = true
		} else {
			refresh = false
		}
		checkAndEnableApply()
	}

	sa.createImageQueryPanel(prefsWindow, prefsPanel, &refresh, checkAndEnableApply)

	footer.Add(widget.NewSeparator())
	footer.Add(applyButton)

	//return wallpaperPrefs
	return prefsPanel
}

// CreatePreferencesWindow creates and displays a new window for the application's preferences.
// The window is titled "Preferences" and is sized to 800x600 pixels, centered on the screen.
// It contains a main container for wallpaper plugin preferences and a close button at the bottom.
// The close button closes the preferences window when clicked.
func (sa *SpiceApp) CreatePreferencesWindow() {
	// Create a new window for the preferences
	prefsWindow := sa.app.NewWindow(fmt.Sprintf("%s Preferences", config.ServiceName))
	prefsWindow.Resize(fyne.NewSize(800, 800))
	prefsWindow.CenterOnScreen()

	// Main Wallpaper Plugin Container
	wallpaperPrefs := sa.createWallpaperPreferences(prefsWindow)
	closeButton := widget.NewButton("Close", func() {
		prefsWindow.Close()
	})

	prefsWindowLayout := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), closeButton), nil, nil, wallpaperPrefs)

	prefsWindow.SetContent(prefsWindowLayout)
	prefsWindow.Show()
}

// verifyEULA checks if the End User License Agreement has been accepted. If not, it will show the EULA and prompt the user to accept it.
// If the user declines, the application will quit.
// If the EULA has been accepted, the application will proceed to setup.
func (sa *SpiceApp) verifyEULA() {
	// Check if the EULA has been accepted
	if util.HasAcceptedEULA() {
		sa.CreateSplashScreen() // Show the splash screen if the EULA has been accepted
	} else {
		sa.displayEULAAcceptance() // Show the EULA if it hasn't been accepted
	}
}

// displayEULAAcceptance displays the End User License Agreement and prompts the user
// to accept it. If the user declines, the application will quit.
func (sa *SpiceApp) displayEULAAcceptance() {
	eulaText, err := sa.assetMgr.GetText("eula.txt")
	if err != nil {
		log.Fatalf("Error loading EULA: %v", err)
	}

	// Create a new window for the EULA
	eulaWindow := sa.app.NewWindow("Spice EULA")
	eulaWindow.Resize(fyne.NewSize(800, 600))
	eulaWindow.CenterOnScreen()
	eulaWindow.SetCloseIntercept(func() {
		// Prevent the window from being closed
	})

	// Create a scrollable text widget for the EULA content
	eulaWdgt := widget.NewRichTextWithText(eulaText)
	eulaWdgt.Wrapping = fyne.TextWrapWord
	eulaScroll := container.NewVScroll(eulaWdgt)
	eulaDialog := dialog.NewCustomConfirm("To continue using Spice, please review and accept the End User License Agreement.", "Accept", "Decline", eulaScroll, func(accepted bool) {
		if accepted {
			// Mark the EULA as accepted
			util.MarkEULAAccepted()
			eulaWindow.Close()
			sa.CreateSplashScreen() // Show the splash screen after user accepts the EULA
		} else {
			// Stop the service before quitting the application
			sa.app.Quit()
		}
	}, eulaWindow)

	eulaDialog.Resize(fyne.NewSize(795, 595)) // Resize the dialog to fit the window
	eulaDialog.Show()
	eulaWindow.Show()
}

// Preferences returns the preferences for the application
func (sa *SpiceApp) Preferences() fyne.Preferences {
	return sa.prefs
}

// Run runs the application
func (sa *SpiceApp) Run() {
	sa.app.Run()
}
