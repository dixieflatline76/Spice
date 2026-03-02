//go:build !linux

package hotkey

import (
	"fmt"
	"time"

	"sync"

	"github.com/dixieflatline76/Spice/v2/pkg/ui"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
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

var (
	registeredHotkeys []*hotkey.Hotkey
	hkMu              sync.Mutex

	// Debouncing and Background Work
	refreshChan = make(chan ui.PluginManager, 1)
	initOnce    sync.Once
)

// initWorker starts a dedicated background goroutine to handle hotkey refreshes.
// This ensures that OS-level hook registration (Carbon/Cocoa/User32) is decoupled
// from the UI thread and debounced to prevent instability.
func initWorker() {
	go func() {
		var debounceTimer *time.Timer
		var latestMgr ui.PluginManager

		for {
			select {
			case mgr := <-refreshChan:
				latestMgr = mgr
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.NewTimer(250 * time.Millisecond)
			case <-func() <-chan time.Time {
				if debounceTimer == nil {
					return nil
				}
				return debounceTimer.C
			}():
				debounceTimer = nil
				if latestMgr != nil {
					doStartListeners(latestMgr)
				}
			}
		}
	}()
}

// StopListeners unregisters all currently active hotkeys.
func StopListeners() {
	hkMu.Lock()
	defer hkMu.Unlock()

	if len(registeredHotkeys) == 0 {
		return
	}

	log.Printf("[Hotkey] Stopping %d listeners and unregistering hooks...", len(registeredHotkeys))
	for _, hk := range registeredHotkeys {
		if err := hk.Unregister(); err != nil {
			log.Debugf("[Hotkey] Failed to unregister during stop: %v", err)
		}
	}
	registeredHotkeys = nil
}

// StartListeners initializes and starts the global hotkey listeners.
func StartListeners(mgr ui.PluginManager) {
	initOnce.Do(initWorker)

	// Push to the background worker to handle debounced registration
	select {
	case refreshChan <- mgr:
	default:
		// Worker is already queued for an update
	}
}

func doStartListeners(mgr ui.PluginManager) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Hotkey] CRITICAL: Recovered from panic during listener registration: %v", r)
		}
	}()

	// First, stop any existing listeners to ensure a clean state if re-initialized
	StopListeners()

	wp := wallpaper.GetInstance()
	if wp != nil && wp.GetShortcutsDisabled() {
		log.Printf("[Hotkey] Global shortcuts are disabled in preferences. Skipping registration.")
		return
	}

	hkMu.Lock()
	defer hkMu.Unlock()

	// --- 2. Global Handlers (Base + Extra Modifier + Key) ---
	// These apply to ALL monitors simultaneously.
	hkGlobalNext := hotkey.New([]hotkey.Modifier{modBase, modExtra}, keyRight)
	hkGlobalPrev := hotkey.New([]hotkey.Modifier{modBase, modExtra}, keyLeft)
	hkGlobalSync := hotkey.New([]hotkey.Modifier{modBase, modExtra}, keyD)
	hkOpts := hotkey.New([]hotkey.Modifier{modBase, modExtra}, keyO)

	// --- 1. Targeted Handlers (Base Modifier + [1-9] + Key) ---
	// These only trigger if a number key 1-9 is held.
	hkTargetedNext := hotkey.New([]hotkey.Modifier{modBase}, keyRight)
	hkTargetedPrev := hotkey.New([]hotkey.Modifier{modBase}, keyLeft)
	hkTargetedTrash := hotkey.New([]hotkey.Modifier{modBase}, keyDown)
	hkTargetedFav := hotkey.New([]hotkey.Modifier{modBase}, keyUp)
	hkTargetedPause := hotkey.New([]hotkey.Modifier{modBase}, keyP)

	// Register Targeted listeners
	registerAndListenTargeted(hkTargetedNext, "Targeted Next", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.SetNextWallpaper(mid, true)
				return fmt.Sprintf("Display %d: Next Wallpaper", mid+1)
			}
			return ""
		})
	})

	registerAndListenTargeted(hkTargetedPrev, "Targeted Previous", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.SetPreviousWallpaper(mid, true)
				return fmt.Sprintf("Display %d: Previous Wallpaper", mid+1)
			}
			return ""
		})
	})

	registerAndListenTargeted(hkTargetedTrash, "Targeted Trash", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.DeleteCurrentImage(mid)
				return fmt.Sprintf("Display %d: Image Blocked", mid+1)
			}
			return ""
		})
	})

	registerAndListenTargeted(hkTargetedFav, "Targeted Favorite", func() {
		handleTargeted(mgr, func(mid int) string {
			if wp != nil {
				wp.TriggerFavorite(mid)
				return "" // Plugin handles its own notifications
			}
			return ""
		})
	})

	registerAndListenTargeted(hkTargetedPause, "Targeted Pause", func() {
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
	log.Debugf("Registered hotkey: %s", name)
	registeredHotkeys = append(registeredHotkeys, hk)

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

func registerAndListenTargeted(hk *hotkey.Hotkey, name string, action func()) {
	wp := wallpaper.GetInstance()
	if wp != nil && (wp.GetTargetedShortcutsDisabled() || wp.GetShortcutsDisabled()) {
		log.Debugf("Skipping targeted hotkey registration for %s (Disabled in Preferences)", name)
		return
	}

	if err := hk.Register(); err != nil {
		log.Printf("Failed to register hotkey %s: %v", name, err)
		return
	}
	log.Debugf("Registered Targeted hotkey: %s", name)
	registeredHotkeys = append(registeredHotkeys, hk)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Hotkey] PANIC in listener loop for %s: %v", name, r)
			}
		}()

		for range hk.Keydown() {
			wp := wallpaper.GetInstance()
			if wp != nil && (wp.GetShortcutsDisabled() || wp.GetTargetedShortcutsDisabled()) {
				continue
			}
			now := time.Now().Format("15:04:05.000")
			log.Debugf("[%s] Targeted hotkey triggered: %s", now, name)
			action()
			// Safety throttle to prevent accidental double-triggers
			time.Sleep(200 * time.Millisecond)
		}
		log.Printf("[Hotkey] Targeted Listener loop exited for %s", name)
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
