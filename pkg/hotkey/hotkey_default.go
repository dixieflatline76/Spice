//go:build !linux

package hotkey

import (
	"fmt"
	"time"

	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
	"golang.design/x/hotkey"
)

// Constants shared by Windows and Darwin (and potentially others in !linux)
const (
	modCtrl  = hotkey.ModCtrl
	modAlt   = hotkey.ModAlt
	modShift = hotkey.ModShift

	keyRight = hotkey.KeyRight
	keyLeft  = hotkey.KeyLeft
	keyUp    = hotkey.KeyUp
	keyDown  = hotkey.KeyDown
	keyP     = hotkey.KeyP
	keyO     = hotkey.KeyO
	keyD     = hotkey.KeyD
)

// GetMonitorIDFromKey is a platform-specific helper.
// On Windows it polls number keys. On Darwin/Linux it's a stub or different impl.
// Since hotkey_windows.go has the real impl, we need a stub here for Darwin?
// No, if hotkey_windows.go has it, and this file is !linux, then on Windows we have TWO generic definitions?
// No, hotkey_windows has the function.
// On Darwin, we need this function to exist.
// So we should define a Stub for it in hotkey_darwin.go, NOT here.
// Or define it here as a var that can be overridden? No.

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
	hkGlobalSync := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, keyD)

	// Start Targeted listeners
	registerAndListen(hkNext, "Targeted Next", func() {
		handleTargeted(mgr, "Next Wallpaper", func(mid int) {
			if wp := wallpaper.GetInstance(); wp != nil {
				wp.SetNextWallpaper(mid, true)
			}
		})
	})

	registerAndListen(hkPrev, "Targeted Previous", func() {
		handleTargeted(mgr, "Previous Wallpaper", func(mid int) {
			if wp := wallpaper.GetInstance(); wp != nil {
				wp.SetPreviousWallpaper(mid, true)
			}
		})
	})

	registerAndListen(hkTrash, "Targeted Trash", func() {
		handleTargeted(mgr, "Image Blocked", func(mid int) {
			if wp := wallpaper.GetInstance(); wp != nil {
				wp.DeleteCurrentImage(mid)
			}
		})
	})

	registerAndListen(hkFav, "Targeted Favorite", func() {
		handleTargeted(mgr, "Added to Favorites", func(mid int) {
			if wp := wallpaper.GetInstance(); wp != nil {
				wp.TriggerFavorite(mid)
			}
		})
	})

	// Start Global listeners
	registerAndListen(hkGlobalNext, "Global Next", func() {
		if wp := wallpaper.GetInstance(); wp != nil {
			wp.SetNextWallpaper(-1, true)
		}
	})

	registerAndListen(hkGlobalPrev, "Global Previous", func() {
		if wp := wallpaper.GetInstance(); wp != nil {
			wp.SetPreviousWallpaper(-1, true)
		}
	})

	// Start Management listeners
	registerAndListen(hkPause, "Toggle Pause", func() {
		if wp := wallpaper.GetInstance(); wp != nil {
			wp.TogglePauseAction()
		}
	})

	registerAndListen(hkOpts, "Open Preferences", func() {
		if wp := wallpaper.GetInstance(); wp != nil {
			wp.TriggerOpenSettings()
		}
	})

	registerAndListen(hkGlobalSync, "Sync Monitors", func() {
		if wp := wallpaper.GetInstance(); wp != nil {
			wp.SyncMonitors(true)
		}
	})
}

func registerAndListen(hk *hotkey.Hotkey, name string, action func()) {
	if err := hk.Register(); err != nil {
		log.Printf("Failed to register hotkey %s: %v", name, err)
		return
	}
	log.Printf("Registered hotkey: %s", name)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Hotkey] PANIC in listener loop for %s: %v", name, r)
			}
		}()

		for range hk.Keydown() {
			now := time.Now().Format("15:04:05.000")
			log.Debugf("[%s] Hotkey triggered: %s", now, name)
			action()
			// Safety throttle to prevent accidental double-triggers
			time.Sleep(200 * time.Millisecond)
		}
		log.Printf("[Hotkey] Listener loop exited for %s", name)
	}()
}

func handleTargeted(mgr ui.PluginManager, actionName string, action func(mid int)) {
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
