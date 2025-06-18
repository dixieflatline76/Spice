//go:build linux
// +build linux

package ui

// implements the OS interface for Linux.
type linuxOS struct{}

// TransformToForeground changes the application to be a regular app with a Dock icon.
func (l *linuxOS) TransformToForeground() {
	// no-op for Linux, as it does not have a Dock like macOS.
}

// TransformToBackground changes the application to be a background-only app.
func (l *linuxOS) TransformToBackground() {
	// no-op for Linux, as it does not have a Dock like macOS.
}

// getOS returns a new instance of the linuxOS struct.
func getOS() OS {
	return &linuxOS{}
}
