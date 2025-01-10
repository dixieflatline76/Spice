package ui

import (
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/service"
	"github.com/dixieflatline76/Spice/wallpaper"
	"golang.org/x/sys/windows/svc"
)

// SpiceApp represents the application
type SpiceApp struct {
	app      fyne.App
	assetMgr *AssetManager
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
				assetMgr: NewAssetManager(),
			}
		})

		instance.CreateTrayMenu()
		instance.CreateSplashScreen()
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
		sa.createMenuItem("Set Wallpaper", func() {
			go wallpaper.SetNextWallpaper()
		}, "next.png"),
		sa.createMenuItem("Previous", func() {
			go wallpaper.SetPreviousWallpaper()
		}, "prev.png"),
		sa.createMenuItem("Random", func() {
			go wallpaper.SetRandomWallpaper()
		}, "rand.png"),
		fyne.NewMenuItemSeparator(), // Divider line
		sa.createMenuItem("Spice", func() {
			go sa.CreateSplashScreen()
		}, "tray.png"),
		fyne.NewMenuItemSeparator(), // Divider line
		sa.createMenuItem("Quit", func() {
			// Stop the service before quitting the application
			service.ControlService(config.ServiceName, svc.Stop, svc.Stopped)
			sa.app.Quit()
		}, "quit.png"),
	)
	desk.SetSystemTrayMenu(trayMenu)
	desk.SetSystemTrayIcon(trayIcon)
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
	splashWindow.CenterOnScreen()
	splashWindow.Show()

	// Hide the splash screen after 2 seconds
	go func() {
		time.Sleep(3 * time.Second)
		splashWindow.Close() // Close the splash window
	}()
}

// Run runs the application
func (sa *SpiceApp) Run() {
	sa.app.Run()
}
