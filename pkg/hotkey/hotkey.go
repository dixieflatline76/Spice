package hotkey

import (
	"fmt"
	"time"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
	"golang.design/x/hotkey"
)

// StartListeners initializes and starts the global hotkey listeners.
// It registers shortcuts for Next, Previous, Trash, Favorites, Pause, and Options.
func StartListeners() {
	// Define shortcuts

	// Navigation & Action (Arrow Cluster)
	// Ctrl + Alt + Right Arrow (Next)
	hkNext := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, hotkey.KeyRight)

	// Ctrl + Alt + Left Arrow (Previous)
	hkPrev := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, hotkey.KeyLeft)

	// Ctrl + Alt + Down Arrow (Trash/Delete)
	hkTrash := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, hotkey.KeyDown)

	// Ctrl + Alt + Up Arrow (Favorite - Strict Add)
	hkFav := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, hotkey.KeyUp)

	// Management (Letter Cluster)
	// Ctrl + Alt + P (Pause/Resume)
	hkPause := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, hotkey.KeyP)

	// Ctrl + Alt + O (Options/Preferences)
	hkOpts := hotkey.New([]hotkey.Modifier{modCtrl, modAlt}, hotkey.KeyO)

	// Helper to register and listen
	registerAndListen := func(hk *hotkey.Hotkey, name string, action func()) {
		if err := hk.Register(); err != nil {
			log.Printf("Failed to register hotkey %s: %v", name, err)
			return
		}
		log.Printf("Registered hotkey: %s", name)

		go func() {
			for range hk.Keydown() {
				log.Debugf("Hotkey pressed: %s", name)
				action()
				// Simple debounce/rate limit
				time.Sleep(200 * time.Millisecond)
			}
		}()
	}

	// Start listeners
	registerAndListen(hkNext, "Global Next", func() {
		go func() {
			wp := wallpaper.GetInstance()
			if wp != nil {
				wp.SetNextWallpaper(-1)
			}
		}()
	})

	registerAndListen(hkPrev, "Global Previous", func() {
		go func() {
			wp := wallpaper.GetInstance()
			if wp != nil {
				wp.SetPreviousWallpaper(-1)
			}
		}()
	})

	registerAndListen(hkTrash, "Trash Wallpaper", func() {
		go func() {
			wp := wallpaper.GetInstance()
			if wp != nil {
				wp.DeleteCurrentImage(0) // Default to primary for hotkey for now
			}
		}()
	})

	registerAndListen(hkFav, "Global Favorite", func() {
		go func() {
			wp := wallpaper.GetInstance()
			if wp != nil {
				wp.TriggerFavorite(-1)
			}
		}()
	})

	// Per-Monitor Next (Alt + [1-9])
	// Note: We register for up to 9 monitors.
	keys := []hotkey.Key{
		hotkey.Key1, hotkey.Key2, hotkey.Key3,
		hotkey.Key4, hotkey.Key5, hotkey.Key6,
		hotkey.Key7, hotkey.Key8, hotkey.Key9,
	}

	for i, k := range keys {
		monitorID := i // Monitor 1 maps to ID 0, etc.
		hkMon := hotkey.New([]hotkey.Modifier{modAlt}, k)
		registerAndListen(hkMon, fmt.Sprintf("Monitor %d Next", i+1), func() {
			go func() {
				wp := wallpaper.GetInstance()
				if wp != nil {
					wp.SetNextWallpaper(monitorID)
				}
			}()
		})
	}

	registerAndListen(hkPause, "Pause/Resume Wallpaper", func() {
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
