package config

import "strings"

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
