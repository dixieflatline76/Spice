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
	trayIcon fyne.Resource
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
		icon, err := fyne.LoadResourceFromPath(config.GetPath() + "/" + config.ServiceName + ".png")
		if err != nil {
			log.Fatalf("Failed to load tray icon: %v", err)
		}
		once.Do(func() {
			instance = &SpiceApp{
				app:      a,
				trayIcon: icon,
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
	si := fyne.NewMenuItem("Spice", func() {
		go sa.CreateSplashScreen()
	})
	si.Icon = sa.trayIcon
	m := fyne.NewMenu(config.ServiceName,
		fyne.NewMenuItem("Next", func() {
			go wallpaper.SetNextWallpaper()
		}),
		fyne.NewMenuItem("Previous", func() {
			go wallpaper.SetPreviousWallpaper()
		}),
		fyne.NewMenuItem("Random", func() {
			go wallpaper.SetRandomWallpaper()
		}),
		fyne.NewMenuItemSeparator(), // Divider line
		si,
		fyne.NewMenuItemSeparator(), // Divider line
		fyne.NewMenuItem("Quit", func() {
			// Stop the service before quitting the application
			service.ControlService(config.ServiceName, svc.Stop, svc.Stopped)
			sa.app.Quit()
		}),
	)
	desk.SetSystemTrayMenu(m)
	desk.SetSystemTrayIcon(sa.trayIcon)
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
	splashImg, err := fyne.LoadResourceFromPath(config.GetPath() + "/splash.png") // Assuming "splash.png" is your image
	if err != nil {
		log.Fatalf("Failed to load splash image: %v", err)
	}

	// Create an image canvas object
	img := canvas.NewImageFromResource(splashImg)
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
