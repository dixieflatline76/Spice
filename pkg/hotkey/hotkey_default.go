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
// modBase and modExtra are defined in platform-specific files.
const (
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
	wp := wallpaper.GetInstance()

	// --- 1. Targeted Handlers (Base Modifier + [1-9] + Key) ---
	// These only trigger if a number key 1-9 is held.
	hkTargetedNext := hotkey.New([]hotkey.Modifier{modBase}, keyRight)
	hkTargetedPrev := hotkey.New([]hotkey.Modifier{modBase}, keyLeft)
	hkTargetedTrash := hotkey.New([]hotkey.Modifier{modBase}, keyDown)
	hkTargetedFav := hotkey.New([]hotkey.Modifier{modBase}, keyUp)
	hkTargetedPause := hotkey.New([]hotkey.Modifier{modBase}, keyP)

	// --- 2. Global Handlers (Base + Extra Modifier + Key) ---
	// These apply to ALL monitors simultaneously.
	hkGlobalNext := hotkey.New([]hotkey.Modifier{modBase, modExtra}, keyRight)
	hkGlobalPrev := hotkey.New([]hotkey.Modifier{modBase, modExtra}, keyLeft)
	hkGlobalSync := hotkey.New([]hotkey.Modifier{modBase, modExtra}, keyD)
	hkOpts := hotkey.New([]hotkey.Modifier{modBase, modExtra}, keyO)

	// Register Targeted listeners
	registerAndListen(hkTargetedNext, "Targeted Next", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.SetNextWallpaper(mid, true)
				return fmt.Sprintf("Display %d: Next Wallpaper", mid+1)
			}
			return ""
		})
	})

	registerAndListen(hkTargetedPrev, "Targeted Previous", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.SetPreviousWallpaper(mid, true)
				return fmt.Sprintf("Display %d: Previous Wallpaper", mid+1)
			}
			return ""
		})
	})

	registerAndListen(hkTargetedTrash, "Targeted Trash", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.DeleteCurrentImage(mid)
				return fmt.Sprintf("Display %d: Image Blocked", mid+1)
			}
			return ""
		})
	})

	registerAndListen(hkTargetedFav, "Targeted Favorite", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.TriggerFavorite(mid)
				return fmt.Sprintf("Display %d: Added to Favorites", mid+1)
			}
			return ""
		})
	})

	registerAndListen(hkTargetedPause, "Targeted Pause", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.TogglePauseMonitorAction(mid)
			}
			return ""
		})
	})

	// Register Global listeners
	registerAndListen(hkGlobalNext, "Global Next", func() {
		if wp != nil {
			wp.SetNextWallpaper(-1, true)
		}
	})

	registerAndListen(hkGlobalPrev, "Global Previous", func() {
		if wp != nil {
			wp.SetPreviousWallpaper(-1, true)
		}
	})

	registerAndListen(hkOpts, "Open Preferences", func() {
		if wp != nil {
			wp.TriggerOpenSettings()
		}
	})

	registerAndListen(hkGlobalSync, "Sync Monitors", func() {
		if wp != nil {
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
			if wp := wallpaper.GetInstance(); wp != nil && wp.GetShortcutsDisabled() {
				continue
			}
			now := time.Now().Format("15:04:05.000")
			log.Debugf("[%s] Hotkey triggered: %s", now, name)
			action()
			// Safety throttle to prevent accidental double-triggers
			time.Sleep(200 * time.Millisecond)
		}
		log.Printf("[Hotkey] Listener loop exited for %s", name)
	}()
}

func handleTargeted(mgr ui.PluginManager, action func(mid int) string) {
	mid := GetMonitorIDFromKey()
	title := "Wallpaper Action"
	msg := ""

	if mid != -1 {
		msg = action(mid)
	} else {
		log.Debugf("[Hotkey] Suppression: Base modifier combination detected but no number key held. Skipping.")
		return
	}

	if mgr != nil && msg != "" {
		go func() {
			log.Debugf("[Hotkey] Dispatching async notification: %s - %s", title, msg)
			mgr.NotifyUser(title, msg)
		}()
	}
}
