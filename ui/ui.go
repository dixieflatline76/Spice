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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/pkg"
	"github.com/dixieflatline76/Spice/util"
)

// SpiceApp represents the application
type SpiceApp struct {
	fyne.App
	assetMgr  *asset.Manager
	trayMenu  *fyne.Menu
	notifiers []pkg.Notifier
	plugins   []pkg.Plugin // List of plugins to activate
}

var (
	saInstance *SpiceApp // Singleton instance of the application
	saOnce     sync.Once // Ensures the singleton is created only once
)

// GetPluginManager returns the singleton instance of the application as a UIPluginManager
func GetPluginManager() pkg.UIPluginManager {
	return getInstance()
}

// Register registers a plugin with the application
func (sa *SpiceApp) Register(plugin pkg.Plugin) {
	sa.plugins = append(sa.plugins, plugin)
	plugin.Init(sa)
}

// Deregister deregisters a plugin from the application
func (sa *SpiceApp) Deregister(plugin pkg.Plugin) {
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
func (sa *SpiceApp) RegisterNotifier(notifier pkg.Notifier) {
	sa.notifiers = append(sa.notifiers, notifier)
}

// GetApplication returns the singleton instance of the application
func GetApplication() pkg.App {
	return getInstance()
}

// GetInstance returns the singleton instance of the application
func getInstance() *SpiceApp {
	// Create a new instance of the application if it doesn't exist
	saOnce.Do(func() {
		a := app.NewWithID(config.AppName)
		if _, ok := a.(desktop.App); ok {

			saInstance = &SpiceApp{
				App:      a,
				assetMgr: asset.NewManager(),
				trayMenu: fyne.NewMenu(config.AppName),
				notifiers: []pkg.Notifier{func(title, message string) {
					a.SendNotification(fyne.NewNotification(title, message))
				}},
				plugins: []pkg.Plugin{},
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
		go sa.CreatePreferencesWindow()
	}, "prefs.png"))
	sa.trayMenu.Items = append(sa.trayMenu.Items, fyne.NewMenuItemSeparator())
	sa.trayMenu.Items = append(sa.trayMenu.Items, sa.CreateMenuItem("About Spice", func() {
		go sa.CreateSplashScreen(aboutSplashTime)
	}, "tray.png"))
	sa.trayMenu.Items = append(sa.trayMenu.Items, fyne.NewMenuItemSeparator())
	sa.trayMenu.Items = append(sa.trayMenu.Items, sa.CreateMenuItem("Quit", func() {
		// Stop the service before quitting the application
		sa.Quit()
	}, "quit.png"))

	desk.SetSystemTrayMenu(sa.trayMenu)
	desk.SetSystemTrayIcon(trayIcon)
	sa.SetIcon(trayIcon)
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
	splashWindow.Show()

	// Hide the splash screen after 3 seconds
	go func() {
		time.Sleep(time.Duration(seconds) * time.Second)
		splashWindow.Close() // Close the splash window
	}()
}

// CreatePreferencesWindow creates and displays a new window for the application's preferences.
// The window is titled "Preferences" and is sized to 800x600 pixels, centered on the screen.
// It contains a main container for wallpaper plugin preferences and a close button at the bottom.
// The close button closes the preferences window when clicked.
func (sa *SpiceApp) CreatePreferencesWindow() {
	// Create a new window for the preferences
	prefsWindow := sa.NewWindow(fmt.Sprintf("%s Preferences", config.AppName))
	prefsWindow.Resize(fyne.NewSize(800, 800))
	prefsWindow.CenterOnScreen()

	prefsContainers := []fyne.CanvasObject{}
	for _, plugin := range sa.plugins {
		prefsContainers = append(prefsContainers, plugin.CreatePrefsPanel(prefsWindow))
	}

	closeButton := widget.NewButton("Close", func() {
		prefsWindow.Close()
	})

	prefsWindowLayout := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), closeButton), nil, nil, prefsContainers...)

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
	eulaWindow := sa.NewWindow("Spice EULA")
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

// Bam activates all plugins and runs the Fyne application
func (sa *SpiceApp) Bam() {

	// Create the tray menu
	saInstance.CreateTrayMenu()

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
