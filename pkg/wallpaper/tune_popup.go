package wallpaper

import (
	"fmt"
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

// showTuneImagePopup gathers state and delegates popup creation to the PluginManager.
// The outer ring (ui/ui.go) owns window lifecycle and OpenGL error recovery.
func (wp *Plugin) showTuneImagePopup(monitorID int) {
	log.Debugf("showTuneImagePopup: Preparing tune image popup for monitor %d", monitorID)

	// Determine current anchor from monitor state
	wp.monMu.RLock()
	mc, ok := wp.Monitors[monitorID]
	wp.monMu.RUnlock()
	if !ok {
		log.Printf("[WARN] showTuneImagePopup: Monitor %d not found", monitorID)
		return
	}

	mc.mu.RLock()
	resKey := fmt.Sprintf("%dx%d", mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy())
	currentOpts := mc.State.CurrentImage.GetTuning(resKey)
	mc.mu.RUnlock()

	if wp.manager == nil {
		log.Printf("[WARN] showTuneImagePopup: No plugin manager available")
		return
	}

	effectiveOpts := currentOpts

	if effectiveOpts.FrameOverride == provider.FrameOverrideInherit {
		if mc.State.CurrentImage.ProcessingFlags["VirtualFramed:"+resKey] {
			effectiveOpts.FrameOverride = provider.FrameOverrideForceOn
		} else {
			effectiveOpts.FrameOverride = provider.FrameOverrideForceOff
		}
	}
	if effectiveOpts.FrameSize == 0 {
		effectiveOpts.FrameSize = wp.cfg.VirtualFrameSize
	}
	if effectiveOpts.Matting == provider.MattingOverrideInherit {
		if wp.cfg.VirtualPaperMatting {
			effectiveOpts.Matting = provider.MattingOverrideOn
		} else {
			effectiveOpts.Matting = provider.MattingOverrideOff
		}
	}
	if effectiveOpts.WallColor == provider.WallColorOverrideInherit {
		if wp.cfg.VirtualWallColor == WallAlgorithmic {
			effectiveOpts.WallColor = provider.WallColorOverrideAlgorithmic
		} else {
			effectiveOpts.WallColor = provider.WallColorOverrideNeutral
		}
	}
	// Determine if the frame should be locked
	lockFrame := false
	if effectiveOpts.FrameOverride == provider.FrameOverrideForceOn {
		// Only lock if the image is actually incompatible with the current SmartFit mode.
		// A perfectly 16:9 museum piece doesn't need to be locked even if framed by museum mode.
		if err := wp.imgProcessor.CheckCompatibility(mc.State.CurrentImage.Width, mc.State.CurrentImage.Height, mc.Monitor.Rect.Dx(), mc.Monitor.Rect.Dy()); err != nil {
			lockFrame = true
		}
	}

	// Send CmdTuningStart to pause the controller
	mc.Commands <- CmdTuningStart

	wp.manager.ShowTuneImagePopup(monitorID, currentOpts, effectiveOpts, anchorLabels, anchorValues, lockFrame,
		func(opts provider.TuningOptions, onDone func()) {
			log.Debugf("showTuneImagePopup: User selected options %v for monitor %d", opts, monitorID)
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

				wp.SetTuningOptions(monitorID, opts)

				// Wait for completion (with timeout to prevent hanging)
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					log.Printf("[WARN] Tuning reprocessing timed out for monitor %d", monitorID)
				}

				if onDone != nil {
					onDone()
				}
			}()
		},
		func() {
			// Resume automatic rotation when popup closes
			mc.Commands <- CmdTuningEnd
		},
	)
}
