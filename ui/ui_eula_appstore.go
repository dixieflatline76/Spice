//go:build appstore

package ui

import (
	"time"

	"github.com/dixieflatline76/Spice/v2/util"
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
)

// verifyEULA checks if the End User License Agreement has been accepted.
// For App Store builds, we auto-accept the EULA and SKIP all window creation (Splash/EULA)
// to ensure a "Headless-First" launch that avoids OpenGL initialization crashes in review VMs.
func (sa *SpiceApp) verifyEULA() {
	utilLog.Print("App Store Build Detected: Auto-accepting EULA and skipping launch windows.")

	// Mark the EULA as accepted if it hasn't been already
	if !util.HasAcceptedEULA(sa.Preferences()) {
		util.MarkEULAAccepted(sa.Preferences())
	}

	// We specifically do NOT call CreateSplashScreen here.
	// This ensures the app starts silently in the tray, avoiding OpenGL window creation
	// until the user (or reviewer) explicitly requests a windowed feature.

	// Hide the Dock icon after Fyne/GLFW finishes initialization.
	// GLFW forces the activation policy to Regular during startup, overriding LSUIElement.
	// In the standard build, CreateSplashScreen handles this via TransformToBackground()
	// after the splash timer. Since we skip splash, we use the lifecycle hook instead.
	sa.Lifecycle().SetOnStarted(func() {
		go func() {
			time.Sleep(200 * time.Millisecond) // Wait for GLFW to finish forcing Regular policy
			sa.os.TransformToBackground()
			utilLog.Print("App Store: Dock icon hidden (activation policy set to Accessory)")
		}()
	})
}
