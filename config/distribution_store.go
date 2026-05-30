//go:build appstore || msstore

package config

// IsStoreDistribution returns true when the binary is built for
// an app store (Apple App Store or Microsoft Store).
func IsStoreDistribution() bool { return true }
