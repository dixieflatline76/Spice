//go:build !darwin
// +build !darwin

// Package fsx holds small filesystem helpers whose implementation is
// platform-specific.
package fsx

// ExcludeFromBackup is a no-op off macOS: there is no equivalent, portable
// "skip this directory" marker for Windows or Linux backup tools.
func ExcludeFromBackup(_ string) error { return nil }
