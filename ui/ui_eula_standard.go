//go:build !appstore

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/util"
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
)

// verifyEULA checks if the End User License Agreement has been accepted. If not, it will show the EULA and prompt the user to accept it.
// If the user declines, the application will quit.
// If the EULA has been accepted, the application will proceed to setup.
func (sa *SpiceApp) verifyEULA() {
	// Check if the EULA has been accepted
	if util.HasAcceptedEULA(sa.Preferences()) {
		sa.CreateSplashScreen(startupSplashTime) // Show the splash screen if the EULA has been accepted
	} else {
		sa.displayEULAAcceptance() // Show the EULA if it hasn't been accepted
	}
}

// displayEULAAcceptance displays the End User License Agreement and prompts the user
// to accept it. If the user declines, the application will quit.
func (sa *SpiceApp) displayEULAAcceptance() {
	eulaText, err := sa.assetMgr.GetText("eula.txt")
	if err != nil {
		utilLog.Fatalf("Error loading EULA: %v", err)
	}

	// Create a new window for the EULA
	sa.os.TransformToForeground() // Ensure the app is in the foreground before showing the EULA
	eulaWindow := sa.NewWindow(i18n.T("Spice EULA"))
	eulaWindow.SetOnClosed(sa.os.TransformToBackground) // Set the close action to transform to background
	eulaWindow.Resize(fyne.NewSize(800, 600))
	eulaWindow.CenterOnScreen()
	eulaWindow.SetCloseIntercept(func() {
		// Prevent the window from being closed
	})

	// Create a scrollable text widget for the EULA content
	eulaWdgt := widget.NewRichTextWithText(eulaText)
	eulaWdgt.Wrapping = fyne.TextWrapWord
	eulaScroll := container.NewVScroll(eulaWdgt)
	eulaDialog := dialog.NewCustomConfirm(i18n.T("To continue using Spice, please review and accept the End User License Agreement."), i18n.T("Accept"), i18n.T("Decline"), eulaScroll, func(accepted bool) {
		if accepted {
			// Mark the EULA as accepted
			util.MarkEULAAccepted(sa.Preferences())
			eulaWindow.Close()
			sa.CreateSplashScreen(startupSplashTime) // Show the splash screen after user accepts the EULA
		} else {
			// Stop the service before quitting the application
			sa.Quit()
		}
	}, eulaWindow)

	eulaDialog.Resize(fyne.NewSize(795, 595)) // Resize the dialog to fit the window
	eulaDialog.Show()
	eulaWindow.Show()
}
