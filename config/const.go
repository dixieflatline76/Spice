package config

import (
	"os"
	"path/filepath"
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
