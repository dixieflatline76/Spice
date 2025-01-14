package ui

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/config"

	"github.com/dixieflatline76/Spice/service"
)

// SpiceApp represents the application
type SpiceApp struct {
	app      fyne.App
	assetMgr *asset.Manager
	trayMenu *fyne.Menu
}

var (
	instance *SpiceApp // Singleton instance of the application
	once     sync.Once // Ensures the singleton is created only once
)

// GetInstance returns the singleton instance of the application
func GetInstance() *SpiceApp {
	// Create a new instance of the application if it doesn't exist
	a := app.NewWithID(config.ServiceName)
	if _, ok := a.(desktop.App); ok {
		once.Do(func() {
			instance = &SpiceApp{
				app:      a,
				assetMgr: asset.NewManager(),
			}
			instance.CreateTrayMenu()
			instance.CheckEULA()
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

// CheckEULA checks if the End User License Agreement has been accepted. If not, it will show the EULA and prompt the user to accept it.
// If the user declines, the application will quit.
// If the EULA has been accepted, the application will proceed to setup.
func (sa *SpiceApp) CheckEULA() {
	eulaAcceptedPath := filepath.Join(config.GetPath(), "eula_accepted")

	// Check if the file exists
	_, err := os.Stat(eulaAcceptedPath)
	if err != nil {
		if os.IsNotExist(err) {
			sa.processEULA() // Show the EULA
		} else {
			log.Fatalf("Failed to check EULA acceptance: %v", err) // Should never happen
		}
	} else {
		sa.CreateSplashScreen() // Show the splash screen if the EULA has been accepted
	}
}

// processEULA displays the End User License Agreement and prompts the user
// to accept it. If the user declines, the application will quit.
func (sa *SpiceApp) processEULA() {
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
			sa.markEULAAccepted()
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

// markEULAAccepted marks the EULA as accepted
func (sa *SpiceApp) markEULAAccepted() {
	eulaAcceptedPath := filepath.Join(config.GetPath(), "eula_accepted")

	// Create an empty file to mark acceptance
	file, err := os.Create(eulaAcceptedPath)
	if err != nil {
		// Handle the error
		return
	}
	defer file.Close()
}

// Run runs the application
func (sa *SpiceApp) Run() {
	sa.app.Run()
}
