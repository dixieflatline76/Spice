//go:build darwin
// +build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AppKit

#import <AppKit/AppKit.h>

// NSApplicationActivationPolicyRegular is a normal, foreground application.
// It has a Dock icon and a menu bar.
const NSApplicationActivationPolicy Regular = 0;

// NSApplicationActivationPolicyAccessory is a background application.
// It has no Dock icon and does not appear in the Force Quit window.
const NSApplicationActivationPolicy Accessory = 1;

// setActivationPolicy activates the application and sets its activation policy.
// This is the magic that makes the Dock icon appear or disappear.
void setActivationPolicy(long policy) {
    [NSApp setActivationPolicy:policy];
    // For the change to take effect, we must activate the app.
    // When transforming to foreground, this brings it forward.
    // When transforming to background, this seems to be necessary to
    // "commit" the policy change.
    [NSApp activateIgnoringOtherApps:YES];
}
*/
import "C"

// linuxOS implements the OS interface for Linux.
type darwinOS struct{}

// TransformToForeground changes the application to be a regular app with a Dock icon.
func (d *darwinOS) TransformToForeground() {
	C.setActivationPolicy(C.Regular)
}

// TransformToBackground changes the application to be a background-only app.
func (d *darwinOS) TransformToBackground() {
	C.setActivationPolicy(C.Accessory)
}

// getOS returns a new instance of the darwinOS struct.
func getOS() OS {
	return &darwinOS{}
}
