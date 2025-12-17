//go:build linux
// +build linux

package ui

import (
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// linuxOS implements the OS interface for Linux.
type linuxOS struct{}

// TransformToForeground changes the application to be a regular app with a Dock icon.
func (l *linuxOS) TransformToForeground() {
	// no-op for Linux
}

// TransformToBackground changes the application to be a background-only app.
func (l *linuxOS) TransformToBackground() {
	// no-op for Linux
}

func (l *linuxOS) SetupLifecycle(app fyne.App, sa *SpiceApp) {
	// No-op for standard Linux
}

// chromeOS implements the OS interface for Chrome OS (Crostini).
type chromeOS struct {
	linuxOS
	trayWindow fyne.Window
}

func (c *chromeOS) SetupLifecycle(app fyne.App, sa *SpiceApp) {
	// On Chrome OS, we use the Dock Icon as a toggle for our "Pseudo-Tray" window.
	// SetOnEnteredForeground is triggered when the app icon is clicked in the shelf.
	app.Lifecycle().SetOnEnteredForeground(func() {
		if c.trayWindow == nil {
			c.createTrayWindow(app, sa)
		}

		if c.trayWindow.Content().Visible() { // Heuristic: check if "visible"
			// Actually Fyne windows don't have IsVisible() easily, but we can track state or just Show/Hide
			// If we can't check, we just Show(). But to TOGGLE, we need state.
			// Let's rely on the window variable being managed.
			// Fyne Window.Hide() makes it invisible.
			// If checking visibility is hard, checking if it is focused might work?
			// Simplest toggle: If it was just created, show it.
			// If we need true toggle, we might need a flag.
			c.trayWindow.Show()
			c.trayWindow.RequestFocus()
		} else {
			c.trayWindow.Show()
			c.trayWindow.RequestFocus()
		}
		// NOTE: True "Toggle" (Hide if showing) is tricky locally without tracking state manually.
		// For now, "Click to Show" is safe.
	})
}

func (c *chromeOS) createTrayWindow(app fyne.App, sa *SpiceApp) {
	w := app.NewWindow("Spice Tray")
	w.SetUndecorated(true)
	w.SetTitle("Spice")

	// Convert Tray Menu items to Buttons
	var items []fyne.CanvasObject
	if sa.trayMenu != nil {
		for _, item := range sa.trayMenu.Items {
			if item.IsSeparator {
				items = append(items, widget.NewSeparator())
				continue
			}
			// Capture loop variable
			menuItem := item
			btn := widget.NewButton(menuItem.Label, func() {
				if menuItem.Action != nil {
					menuItem.Action()
				}
				// Auto-close tray window after action (except maybe "Next Wallpaper"?)
				// Use user preference or default behavior. For native tray, it closes.
				w.Hide()
			})
			items = append(items, btn)
		}
	}

	content := container.NewVBox(items...)
	w.SetContent(container.NewPadded(content))

	// Positioning: Try to place at bottom right.
	// Note: Wayland often ignores this, but we try.
	// Hardcoded fallback logic or just center.
	w.CenterOnScreen() // Start centered, safety first.

	c.trayWindow = w
}

// getOS returns a new instance of the OS struct.
func getOS() OS {
	// Simple check for Chrome OS environment marker
	if _, err := os.Stat("/dev/.cros_milestone"); err == nil {
		return &chromeOS{}
	}
	return &linuxOS{}
}
