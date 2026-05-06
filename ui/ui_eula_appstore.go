//go:build appstore

package ui

import (
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
}
