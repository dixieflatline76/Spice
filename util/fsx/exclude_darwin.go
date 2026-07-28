//go:build darwin
// +build darwin

// Package fsx holds small filesystem helpers whose implementation is
// platform-specific.
package fsx

import (
	"golang.org/x/sys/unix"
)

// backupExcludeAttr is the extended attribute Time Machine honours to skip a
// directory. The value is a binary plist holding the boolean true.
const backupExcludeAttr = "com.apple.metadata:com_apple_backup_excludeItem"

// backupExcludeValue is bplist00 followed by the archived string
// "com.apple.backupd", the marker Time Machine writes for its own exclusions.
var backupExcludeValue = []byte("bplist00_\x10\x11com.apple.backupd\x08\x00\x00\x00\x00\x00\x00\x01\x01\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x1c")

// ExcludeFromBackup marks a directory so Time Machine skips it.
//
// Wallpaper images are re-downloadable and can run to several gigabytes;
// backing them up wastes the user's backup space for no benefit. This is
// best-effort: an error here is never worth failing a startup over.
func ExcludeFromBackup(path string) error {
	return unix.Setxattr(path, backupExcludeAttr, backupExcludeValue, 0)
}
