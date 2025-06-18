//go:build windows
// +build windows

package ui

// implements the OS interface for Linux.
type windowsOS struct{}

// TransformToForeground changes the application to be a regular app with a Dock icon.
func (w *windowsOS) TransformToForeground() {
	// no-op for Windows, as it does not have a Dock like macOS.
}

// TransformToBackground changes the application to be a background-only app.
func (w *windowsOS) TransformToBackground() {
	// no-op for Windows, as it does not have a Dock like macOS.
}

// getOS returns a new instance of the windowsOS struct.
func getOS() OS {
	return &windowsOS{}
}
