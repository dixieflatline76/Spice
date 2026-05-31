//go:build !windows
// +build !windows

package wallpaper

// resolveMSIXPath is a no-op on non-Windows platforms.
// MSIX packaging only exists on Windows.
func resolveMSIXPath(p string) string { return p }
