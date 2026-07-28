//go:build !windows
// +build !windows

package wallpaper

// MSIX packaging only exists on Windows.
// Staging functions are no-ops on non-Windows platforms.
func resolveMSIXPicturePath(p string) string  { return p }
func resolveMSIXDocumentPath(p string) string { return p }
func resolveMSIXPath(p string) string         { return p }
