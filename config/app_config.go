package config

import "fyne.io/fyne/v2"

// AppNotificationsEnabledKey is the key for the app notifications enabled preference
const AppNotificationsEnabledKey = "app_notifications_enabled"

// AppConfig holds the application-wide configuration
type AppConfig struct {
	prefs fyne.Preferences
}

// NewAppConfig creates a new AppConfig instance
func NewAppConfig(p fyne.Preferences) *AppConfig {
	return &AppConfig{prefs: p}
}

// GetAppNotificationsEnabled returns whether system notifications are enabled
func (c *AppConfig) GetAppNotificationsEnabled() bool {
	return c.prefs.BoolWithFallback(AppNotificationsEnabledKey, true)
}

// SetAppNotificationsEnabled sets whether system notifications are enabled
func (c *AppConfig) SetAppNotificationsEnabled(enabled bool) {
	c.prefs.SetBool(AppNotificationsEnabledKey, enabled)
}
