//go:build !windows

package main

// suppressCrashDialogs is a no-op on non-Windows platforms.
// macOS and Linux do not show crash dialogs for child processes.
func suppressCrashDialogs() {}
