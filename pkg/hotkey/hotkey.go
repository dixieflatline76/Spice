package hotkey

import (
	"fmt"
	"time"

	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
	"golang.design/x/hotkey"
)

// StartListeners initializes and starts the global hotkey listeners.
// It registers shortcuts for Next, Previous, Trash, Favorites, Pause, and Options.
func StartListeners(mgr ui.PluginManager) {
	// Define shortcuts

	// --- 1. Targeted Handlers (Opt/Alt + Arrows) ---
	hkNext := hotkey.New([]hotkey.Modifier{modAlt}, keyRight)
	hkPrev := hotkey.New([]hotkey.Modifier{modAlt}, keyLeft)
	hkTrash := hotkey.New([]hotkey.Modifier{modAlt}, keyDown)
	hkFav := hotkey.New([]hotkey.Modifier{modAlt}, keyUp)

	// --- 2. Global Handlers (Cmd+Opt / Ctrl+Alt + Arrows) ---
	hkGlobalNext := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, keyRight)
	hkGlobalPrev := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, keyLeft)

	// --- 3. Management (Cmd+Opt / Ctrl+Alt + Letters) ---
	hkPause := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, keyP)
	hkOpts := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, keyO)

	// Helper to register and listen
	registerAndListen := func(hk *hotkey.Hotkey, name string, action func()) {
		if err := hk.Register(); err != nil {
			log.Printf("Failed to register hotkey %s: %v", name, err)
			return
		}
		log.Printf("Registered hotkey: %s", name)

		go func() {
			for range hk.Keydown() {
				now := time.Now().Format("15:04:05.000")
				log.Debugf("[%s] Hotkey detected: %s", now, name)
				action()
				time.Sleep(200 * time.Millisecond)
			}
		}()
	}

	// Targeted Actions Logic
	handleTargeted := func(actionName string, action func(mid int)) {
		mid := GetMonitorIDFromKey()
		title := "Wallpaper Action"
		msg := ""

		if mid != -1 {
			msg = fmt.Sprintf("Display %d: %s", mid+1, actionName)
			action(mid)
		} else {
			// Default to display 1 if no number key is held
			msg = fmt.Sprintf("Display 1: %s", actionName)
			action(0)
		}

		if mgr != nil {
			go func() {
				log.Debugf("[Hotkey] Dispatching async notification: %s - %s", title, msg)
				mgr.NotifyUser(title, msg)
			}()
		}
	}

	// Start Targeted listeners
	registerAndListen(hkNext, "Targeted Next", func() {
		handleTargeted("Next Wallpaper", func(mid int) {
			wp := wallpaper.GetInstance()
			if wp != nil {
				wp.SetNextWallpaper(mid, true)
			}
		})
	})

	registerAndListen(hkPrev, "Targeted Previous", func() {
		handleTargeted("Previous Wallpaper", func(mid int) {
			wp := wallpaper.GetInstance()
			if wp != nil {
				wp.SetPreviousWallpaper(mid, true)
			}
		})
	})

	registerAndListen(hkTrash, "Targeted Trash", func() {
		handleTargeted("Image Blocked", func(mid int) {
			wp := wallpaper.GetInstance()
			if wp != nil {
				wp.DeleteCurrentImage(mid)
			}
		})
	})

	registerAndListen(hkFav, "Targeted Favorite", func() {
		handleTargeted("Added to Favorites", func(mid int) {
			wp := wallpaper.GetInstance()
			if wp != nil {
				wp.TriggerFavorite(mid)
			}
		})
	})

	// Start Global listeners
	registerAndListen(hkGlobalNext, "Global Next", func() {
		wp := wallpaper.GetInstance()
		if wp != nil {
			wp.SetNextWallpaper(-1, true)
		}
		if mgr != nil {
			mgr.NotifyUser("Spice Global", "Refreshed All Displays")
		}
	})

	registerAndListen(hkGlobalPrev, "Global Previous", func() {
		wp := wallpaper.GetInstance()
		if wp != nil {
			wp.SetPreviousWallpaper(-1, true)
		}
		if mgr != nil {
			mgr.NotifyUser("Spice Global", "Restored All Displays")
		}
	})

	// Management
	registerAndListen(hkPause, "Pause/Resume", func() {
		wp := wallpaper.GetInstance()
		if wp != nil {
			wp.TogglePauseAction()
		}
	})

	registerAndListen(hkOpts, "Open Preferences", func() {
		wp := wallpaper.GetInstance()
		if wp != nil {
			wp.TriggerOpenSettings()
		}
	})
}
