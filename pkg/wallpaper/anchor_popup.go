package wallpaper

import (
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// anchorLabels maps each grid position to its display label.
var anchorLabels = [9]string{"↖", "↑", "↗", "←", "●", "→", "↙", "↓", "↘"}

// anchorValues maps each grid index to its provider.CropAnchor value.
var anchorValues = [9]provider.CropAnchor{
	provider.AnchorTopLeft, provider.AnchorTopCenter, provider.AnchorTopRight,
	provider.AnchorMiddleLeft, provider.AnchorMiddleCenter, provider.AnchorMiddleRight,
	provider.AnchorBottomLeft, provider.AnchorBottomCenter, provider.AnchorBottomRight,
}

// showAnchorPopup gathers state and delegates popup creation to the PluginManager.
// The outer ring (ui/ui.go) owns window lifecycle and OpenGL error recovery.
func (wp *Plugin) showAnchorPopup(monitorID int) {
	log.Debugf("showAnchorPopup: Preparing anchor popup for monitor %d", monitorID)

	// Determine current anchor from monitor state
	wp.monMu.RLock()
	mc, ok := wp.Monitors[monitorID]
	wp.monMu.RUnlock()
	if !ok {
		log.Printf("[WARN] showAnchorPopup: Monitor %d not found", monitorID)
		return
	}

	mc.mu.RLock()
	currentAnchor := mc.State.CurrentImage.CropAnchor
	mc.mu.RUnlock()

	if wp.manager == nil {
		log.Printf("[WARN] showAnchorPopup: No plugin manager available")
		return
	}

	wp.manager.ShowAnchorPopup(monitorID, currentAnchor, anchorLabels, anchorValues,
		func(anchor provider.CropAnchor, onDone func()) {
			log.Debugf("showAnchorPopup: User selected anchor %v for monitor %d", anchor, monitorID)
			go func() {
				// Hook into OnWallpaperChanged to detect when reprocessing completes
				done := make(chan struct{}, 1)
				wp.monMu.RLock()
				mc, ok := wp.Monitors[monitorID]
				wp.monMu.RUnlock()

				if ok {
					origCallback := mc.OnWallpaperChanged
					mc.OnWallpaperChanged = func(img provider.Image, mID int) {
						// Restore original callback first
						mc.OnWallpaperChanged = origCallback
						// Signal done
						select {
						case done <- struct{}{}:
						default:
						}
						// Call the original callback
						if origCallback != nil {
							origCallback(img, mID)
						}
					}
				}

				wp.SetCropAnchor(monitorID, anchor)

				// Wait for completion (with timeout to prevent hanging)
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					log.Printf("[WARN] Anchor reprocessing timed out for monitor %d", monitorID)
				}

				if onDone != nil {
					onDone()
				}
			}()
		},
	)
}
