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

// DebugLoggingEnabledKey is the key for the debug logging preference
const DebugLoggingEnabledKey = "debug_logging_enabled"

// GetDebugLoggingEnabled returns whether debug logging is enabled
func (c *AppConfig) GetDebugLoggingEnabled() bool {
	return c.prefs.BoolWithFallback(DebugLoggingEnabledKey, false)
}

// SetDebugLoggingEnabled sets whether debug logging is enabled
func (c *AppConfig) SetDebugLoggingEnabled(enabled bool) {
	c.prefs.SetBool(DebugLoggingEnabledKey, enabled)
}

// AppLanguageKey is the key for the app language preference
const AppLanguageKey = "app_language"

// GetLanguage returns the current application language override.
// Returns "System" to use the system locale, or a specific locale code (e.g. "en", "de").
func (c *AppConfig) GetLanguage() string {
	return c.prefs.StringWithFallback(AppLanguageKey, "System")
}

// SetLanguage sets the application language override.
func (c *AppConfig) SetLanguage(lang string) {
	c.prefs.SetString(AppLanguageKey, lang)
}
