package hotkey

import (
	"time"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
	"golang.design/x/hotkey"
)

// StartListeners initializes and starts the global hotkey listeners.
// It registers shortcuts for Next, Previous, Trash, and Pause.
func StartListeners() {
	// Define shortcuts
	// Ctrl + Alt + Right Arrow (Next)
	hkNext := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModAlt}, hotkey.KeyRight)

	// Ctrl + Alt + Left Arrow (Previous)
	hkPrev := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModAlt}, hotkey.KeyLeft)

	// Ctrl + Alt + Down Arrow (Trash/Delete)
	hkTrash := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModAlt}, hotkey.KeyDown)

	// Ctrl + Alt + Up Arrow (Pause/Resume)
	hkPause := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModAlt}, hotkey.KeyUp)

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
				// Simple debounce/rate limit if needed, though the channel handles it reasonably well
				time.Sleep(200 * time.Millisecond)
			}
		}()
	}

	// Start listeners
	registerAndListen(hkNext, "Next Wallpaper", func() {
		wp := wallpaper.GetInstance()
		if wp != nil {
			wp.SetNextWallpaper()
		}
	})

	registerAndListen(hkPrev, "Previous Wallpaper", func() {
		wp := wallpaper.GetInstance()
		if wp != nil {
			wp.SetPreviousWallpaper()
		}
	})

	registerAndListen(hkTrash, "Trash Wallpaper", func() {
		wp := wallpaper.GetInstance()
		if wp != nil {
			wp.DeleteCurrentImage()
		}
	})

	registerAndListen(hkPause, "Pause/Resume Wallpaper", func() {
		wp := wallpaper.GetInstance()
		if wp != nil {
			wp.TogglePauseAction()
		}
	})
}
