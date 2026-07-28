//go:build !windows
// +build !windows

package wallpaper

// MSIX packaging only exists on Windows.
// Variable declarations and staging functions are no-ops on non-Windows platforms.

var (
	msixPackageFamilyName string
	msixPictureStaging    string
	msixDocumentStaging   string
)

func resolveMSIXPicturePath(p string) string  { return p }
func resolveMSIXDocumentPath(p string) string { return p }
func resolveMSIXPath(p string) string         { return p }
