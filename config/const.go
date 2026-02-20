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
	} else {
		userHomeDir, _ := os.UserHomeDir()
		appDir = filepath.Join(userHomeDir, "."+strings.ToLower(AppName))
	}
	if err := os.MkdirAll(appDir, 0755); err != nil {
		// We use standard library log here because util/log depends on this package
		panic("failed to create application directory: " + err.Error())
	}
}

// GetAppDir returns the persistent application directory for config and logs.
func GetAppDir() string {
	return appDir
}
