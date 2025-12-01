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

// AppUpdateCheckEnabledKey is the key for the app update check enabled preference
const AppUpdateCheckEnabledKey = "app_update_check_enabled"

// GetUpdateCheckEnabled returns whether the application should check for updates
func (c *AppConfig) GetUpdateCheckEnabled() bool {
	return c.prefs.BoolWithFallback(AppUpdateCheckEnabledKey, true)
}

// SetUpdateCheckEnabled sets whether the application should check for updates
func (c *AppConfig) SetUpdateCheckEnabled(enabled bool) {
	c.prefs.SetBool(AppUpdateCheckEnabledKey, enabled)
}

// AppThemeKey is the key for the app theme preference
const AppThemeKey = "app_theme"

// GetTheme returns the current application theme
func (c *AppConfig) GetTheme() string {
	return c.prefs.StringWithFallback(AppThemeKey, "System")
}

// SetTheme sets the application theme
func (c *AppConfig) SetTheme(theme string) {
	c.prefs.SetString(AppThemeKey, theme)
}
