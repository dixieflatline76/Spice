package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AppVersion is the version of the service.
var AppVersion string // Or get it from version.txt during build

// AppName is the name of the service.
const AppName = "Spice"

// LogWinSubDir is the sub directory for the log files on windows.
var LogWinSubDir = AppName

// LogSubDir is the sub directory for the log files.
var LogSubDir = "." + strings.ToLower(AppName)

// LogExt is the extension for the log files.
var LogExt = ".log"

// GetWorkingDir returns the working directory for the service.
func GetWorkingDir() string {
	return filepath.Join(os.TempDir(), strings.ToLower(AppName))
}

var appDir string

func init() {
	if runtime.GOOS == "windows" {
		userCacheDir, _ := os.UserCacheDir()
		appDir = filepath.Join(userCacheDir, AppName)
	} else if runtime.GOOS == "darwin" {
		// macOS: Use standard Application Support for sandbox compliance.
		// os.UserConfigDir() correctly points to the sandbox container when enabled.
		configDir, err := os.UserConfigDir()
		if err == nil {
			appDir = filepath.Join(configDir, AppName)
		} else {
			// Fallback to home dir if config dir is unavailable
			userHomeDir, _ := os.UserHomeDir()
			appDir = filepath.Join(userHomeDir, "."+strings.ToLower(AppName))
		}
	} else {
		userHomeDir, _ := os.UserHomeDir()
		appDir = filepath.Join(userHomeDir, "."+strings.ToLower(AppName))
	}

	// We don't panic here because it crashes the app before logging starts.
	// We'll attempt to create it, but failures will be handled/logged later
	// when the app tries to actually write to it.
	_ = os.MkdirAll(appDir, 0755)
}

// GetAppDir returns the persistent application directory for config and logs.
func GetAppDir() string {
	return appDir
}
