package ui

import (
	"fmt"
	"image"
	"image/color"
	"net/url"
	"strings"
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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/pkg/sysinfo"
	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/util"
	"github.com/dixieflatline76/Spice/util/log"
)

// SpiceApp represents the application
type SpiceApp struct {
	fyne.App
	assetMgr  *asset.Manager
	trayMenu  *fyne.Menu
	splash    fyne.Window   // Splash window for initial setup
	notifiers []ui.Notifier // List of notifiers to activate
	plugins   []ui.Plugin   // List of plugins to activate
	os        OS            // Operating system interface
}

// OS interface defines methods for transforming the application state
// to foreground or background. This is useful for managing the application
// behavior on different operating systems, such as macOS where background apps
// do not show a Dock icon.
type OS interface {
	// TransformToForeground changes the application to be a regular app with a Dock icon.
	TransformToForeground()
	// TransformToBackground changes the application to be a background-only app.
	TransformToBackground()
}

var (
	saInstance *SpiceApp // Singleton instance of the application
	saOnce     sync.Once // Ensures the singleton is created only once
)

// GetPluginManager returns the singleton instance of the application as a UIPluginManager
func GetPluginManager() ui.PluginManager {
	return getInstance()
}

// Register registers a plugin with the application
func (sa *SpiceApp) Register(plugin ui.Plugin) {
	sa.plugins = append(sa.plugins, plugin)
	plugin.Init(sa)
}

// Deregister deregisters a plugin from the application
func (sa *SpiceApp) Deregister(plugin ui.Plugin) {
	// Implementation for deregistering a plugin
	for i, p := range sa.plugins {
		if p == plugin {
			sa.plugins = append(sa.plugins[:i], sa.plugins[i+1:]...)
			break
		}
	}
}

// GetPreferences returns the preferences for the application
func (sa *SpiceApp) GetPreferences() fyne.Preferences {
	return sa.Preferences()
}

// RegisterNotifier registers a notifier with the application
func (sa *SpiceApp) RegisterNotifier(notifier ui.Notifier) {
	sa.notifiers = append(sa.notifiers, notifier)
}

// GetApplication returns the singleton instance of the application
func GetApplication() ui.App {
	return getInstance()
}

// GetInstance returns the singleton instance of the application
func getInstance() *SpiceApp {
	// Create a new instance of the application if it doesn't exist
	saOnce.Do(func() {
		// Initialize the wallpaper service for right OS
		currentOS := getOS()

		a := app.NewWithID(config.AppName)
		if _, ok := a.(desktop.App); ok {

			saInstance = &SpiceApp{
				App:      a,
				assetMgr: asset.NewManager(),
				trayMenu: fyne.NewMenu(config.AppName),
				notifiers: []ui.Notifier{func(title, message string) {
					a.SendNotification(fyne.NewNotification(title, message))
				}},
				plugins: []ui.Plugin{},
				os:      currentOS,
			}
			saInstance.verifyEULA()
		} else {
			log.Fatal("Spice not supported on this platform")
		}
	})
	return saInstance
}

// NotifyUser sends a notification to the user via all registered notifiers
func (sa *SpiceApp) NotifyUser(title, message string) {
	for _, notify := range sa.notifiers {
		notify(title, message)
	}
}

// CreateTrayMenu creates the tray menu for the application
func (sa *SpiceApp) CreateTrayMenu() {
	desk := sa.App.(desktop.App)
	trayIcon, _ := sa.assetMgr.GetIcon("tray.png")
	for i, plugin := range sa.plugins {
		if i == 0 {
			sa.trayMenu.Items = append(sa.trayMenu.Items, plugin.CreateTrayMenuItems()...)
			sa.trayMenu.Items = append(sa.trayMenu.Items, fyne.NewMenuItemSeparator())
		} else {
			pluginSubmenu := fyne.NewMenuItem(plugin.Name(), nil)
			pluginSubmenu.ChildMenu.Label = plugin.Name()
			pluginSubmenu.ChildMenu.Items = plugin.CreateTrayMenuItems()
			sa.trayMenu.Items = append(sa.trayMenu.Items, pluginSubmenu)
		}
	}

	sa.trayMenu.Items = append(sa.trayMenu.Items, sa.CreateMenuItem("Preferences", func() {
		sa.CreatePreferencesWindow()
	}, "prefs.png"))
	sa.trayMenu.Items = append(sa.trayMenu.Items, fyne.NewMenuItemSeparator())
	sa.trayMenu.Items = append(sa.trayMenu.Items, sa.CreateMenuItem("About Spice", func() {
		sa.CreateSplashScreen(aboutSplashTime)
	}, "tray.png"))
	sa.trayMenu.Items = append(sa.trayMenu.Items, fyne.NewMenuItemSeparator())
	sa.trayMenu.Items = append(sa.trayMenu.Items, sa.CreateMenuItem("Quit", func() {
		sa.os.TransformToForeground()     // Ensure the app is in the foreground before quitting
		time.Sleep(50 * time.Millisecond) // Small delay to ensure the OS processes the state change
		sa.deactivateAllPlugins()         // Deactivate all plugins before quitting
		time.Sleep(2 * time.Second)       // Small delay to ensure plugins are deactivate
		log.Println("Quitting Spice application")
		sa.Quit()
	}, "quit.png"))

	sa.SetIcon(trayIcon)
	desk.SetSystemTrayMenu(sa.trayMenu)
	desk.SetSystemTrayIcon(trayIcon)
}

// DeactivateAllPlugins deactivates all plugins in the application
func (sa *SpiceApp) deactivateAllPlugins() {
	// Deactivate all plugins
	for _, plugin := range sa.plugins {
		plugin.Deactivate()
		log.Printf("Deactivated plugin: %s", plugin.Name())
	}
}

// CreateMenuItem creates a menu item with the given label, action, and icon
func (sa *SpiceApp) CreateMenuItem(label string, action func(), iconName string) *fyne.MenuItem {
	mi := fyne.NewMenuItem(label, action)
	icon, err := sa.assetMgr.GetIcon(iconName)
	if err != nil {
		log.Printf("Failed to load icon: %v", err)
		return mi
	}
	mi.Icon = icon
	return mi
}

// CreateToggleMenuItem creates a toggle menu item with the given label, action, icon, and checked state
func (sa *SpiceApp) CreateToggleMenuItem(label string, action func(bool), iconName string, checked bool) *fyne.MenuItem {

	mi := fyne.NewMenuItem("", nil)

	if checked {
		mi.Label = fmt.Sprintf("%s ✔", label)
	} else {
		mi.Label = label
	}

	icon, err := sa.assetMgr.GetIcon(iconName)
	if err != nil {
		log.Printf("Failed to load icon: %v", err)
		return mi
	}

	mi.Icon = icon
	mi.Checked = checked
	mi.Action = func() {
		newChecked := !mi.Checked
		if newChecked {
			mi.Label = fmt.Sprintf("%s ✔", label)
		} else {
			mi.Label = label
		}
		mi.Checked = newChecked
		action(newChecked)
		sa.trayMenu.Refresh()
	}

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
func (sa *SpiceApp) CreateSplashScreen(seconds int) {
	if sa.splash == nil {
		// Create a splash screen with the application icon
		drv, ok := sa.Driver().(desktop.Driver)
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
		sa.splash = splashWindow
	}

	// Show the splash screen immediately after creation
	sa.splash.Show()

	// Hide the splash screen after 3 seconds
	fyne.Do(
		func() {
			time.Sleep(time.Duration(seconds) * time.Second)
			sa.splash.Hide()              // Close the splash window
			sa.os.TransformToBackground() // Transform the app to background state
		})
}

// CreatePreferencesWindow creates and displays a new window for the application's preferences.
// The window is titled "Preferences" and is sized to 800x600 pixels, centered on the screen.
// It contains a main container for wallpaper plugin preferences and a close button at the bottom.
// The close button closes the preferences window when clicked.
func (sa *SpiceApp) CreatePreferencesWindow() {
	// Create a new window for the preferences
	sa.os.TransformToForeground()
	prefsWindow := sa.NewWindow(fmt.Sprintf("%s Preferences", config.AppName))

	// Set window size based on screen dimensions
	_, height, err := sysinfo.GetScreenDimensions()
	if err != nil {
		log.Printf("Failed to get screen dimensions: %v", err)
		prefsWindow.Resize(fyne.NewSize(800, 600)) // Fallback size
	} else {
		targetHeight := float32(height) * PreferencesWindowHeightRatio
		targetWidth := targetHeight * PreferencesWindowWidthRatio
		prefsWindow.Resize(fyne.NewSize(targetWidth, targetHeight))
	}

	prefsWindow.CenterOnScreen()
	prefsWindow.SetOnClosed(sa.os.TransformToBackground)
	sm := NewSettingsManager(prefsWindow)

	prefsContainers := []fyne.CanvasObject{}
	for _, plugin := range sa.plugins {
		prefsContainers = append(prefsContainers, plugin.CreatePrefsPanel(sm))
	}

	closeButton := widget.NewButton("Close", func() {
		prefsWindow.Close()
	})

	prefsWindowLayout := container.NewBorder(nil, container.NewVBox(sm.GetApplySettingsButton(), container.NewHBox(layout.NewSpacer(), closeButton)), nil, nil, prefsContainers...)

	prefsWindow.SetContent(prefsWindowLayout)
	prefsWindow.Show()
}

// verifyEULA checks if the End User License Agreement has been accepted. If not, it will show the EULA and prompt the user to accept it.
// If the user declines, the application will quit.
// If the EULA has been accepted, the application will proceed to setup.
func (sa *SpiceApp) verifyEULA() {
	// Check if the EULA has been accepted
	if util.HasAcceptedEULA(sa.Preferences()) {
		sa.CreateSplashScreen(startupSplashTime) // Show the splash screen if the EULA has been accepted
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
	sa.os.TransformToForeground() // Ensure the app is in the foreground before showing the EULA
	eulaWindow := sa.NewWindow("Spice EULA")
	eulaWindow.SetOnClosed(sa.os.TransformToBackground) // Set the close action to transform to background
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
			util.MarkEULAAccepted(sa.Preferences())
			eulaWindow.Close()
			sa.CreateSplashScreen(startupSplashTime) // Show the splash screen after user accepts the EULA
		} else {
			// Stop the service before quitting the application
			sa.Quit()
		}
	}, eulaWindow)

	eulaDialog.Resize(fyne.NewSize(795, 595)) // Resize the dialog to fit the window
	eulaDialog.Show()
	eulaWindow.Show()
}

// Start activates all plugins and runs the Fyne application
func (sa *SpiceApp) Start() {

	// Create the tray menu
	saInstance.CreateTrayMenu()
	go sa.StartPeriodicUpdateCheck()

	// Activate all plugins
	go func() {
		time.Sleep(500 * time.Millisecond) // Wait for the tray menu to be created and the ui to be ready
		for _, plugin := range sa.plugins {
			plugin.Activate()
			log.Printf("Activated plugin: %s", plugin.Name())
		}
	}()

	// Run the Fyne application
	sa.Run()
}

// StartPeriodicUpdateCheck starts a goroutine to check for updates on startup and then periodically.
func (sa *SpiceApp) StartPeriodicUpdateCheck() {
	log.Print("Starting periodic application update checker...")

	performCheck := func() {
		log.Print("Checking for application updates...")
		updateInfo, err := util.CheckForUpdates()
		if err != nil {
			log.Printf("Update check failed: %v", err)
			return
		}
		sa.updateTrayMenu(updateInfo)
	}

	time.Sleep(1 * time.Minute)
	performCheck()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		performCheck()
	}
}

// updateTrayMenu adds or removes the "Update Available" menu item based on check results.
func (sa *SpiceApp) updateTrayMenu(info *util.CheckForUpdatesResult) {
	// First, create a new slice of items that excludes any previous update item.
	newItems := make([]*fyne.MenuItem, 0)
	for _, item := range sa.trayMenu.Items {
		if !strings.HasPrefix(item.Label, updateMenuItemPrefix) {
			newItems = append(newItems, item)
		}
	}
	sa.trayMenu.Items = newItems

	// If no update is available, we're done.
	if !info.UpdateAvailable {
		sa.trayMenu.Refresh()
		return
	}

	// If an update IS available, create the new menu item.
	releaseURL, err := url.Parse(info.ReleaseURL)
	if err != nil {
		log.Printf("Could not parse release URL for update menu item: %v", err)
		return
	}
	log.Printf("New version available: %s", info.LatestVersion)

	updateItem := sa.CreateMenuItem(
		fmt.Sprintf("%s%s", updateMenuItemPrefix, info.LatestVersion),
		func() { sa.OpenURL(releaseURL) },
		"download.png",
	)

	// Find the insertion index (just before the separator above "About Spice").
	insertIndex := -1
	for i, item := range sa.trayMenu.Items {
		if item.Label == "About Spice" {
			if i > 0 && sa.trayMenu.Items[i-1].IsSeparator {
				insertIndex = i - 1
			} else {
				insertIndex = i
			}
			break
		}
	}

	// Insert the new item at the correct position.
	if insertIndex != -1 {
		sa.trayMenu.Items = append(sa.trayMenu.Items[:insertIndex], append([]*fyne.MenuItem{updateItem}, sa.trayMenu.Items[insertIndex:]...)...)
	} else {
		// Fallback: add before the Quit button's separator if "About" isn't found.
		for i, item := range sa.trayMenu.Items {
			if item.Label == "Quit" {
				insertIndex = i - 1
				break
			}
		}
		if insertIndex != -1 {
			sa.trayMenu.Items = append(sa.trayMenu.Items[:insertIndex], append([]*fyne.MenuItem{updateItem}, sa.trayMenu.Items[insertIndex:]...)...)
		}
	}

	// Notify user of new version available
	sa.NotifyUser("Spice: Update Available", fmt.Sprintf("Click the tray icon to download version %s.", info.LatestVersion))

	sa.trayMenu.Refresh()
}
