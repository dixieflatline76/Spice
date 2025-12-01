//go:build release

package log

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dixieflatline76/Spice/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

func init() {
	// Determine the log directory based on the OS
	var logDir string
	if runtime.GOOS == "windows" {
		// Use os.UserCacheDir() for Windows as well
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			log.Fatalf("Failed to get user cache directory: %v", err)
		}
		logDir = filepath.Join(userCacheDir, config.LogWinSubDir)
	} else {
		userHomeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get user home directory: %v", err)
		}
		logDir = filepath.Join(userHomeDir, config.LogSubDir)
	}

	// Ensure the log directory exists
	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		log.Fatalf("Failed to create log directory: %v", err)
	}

	// Construct the log file path
	logFilePath := filepath.Join(logDir, config.AppName+config.LogExt)

	log.SetOutput(&lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10, // MB
		MaxBackups: 2,
		MaxAge:     28, // days
		Compress:   true,
	})
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

// Print calls the standard log.Print()
func Print(v ...interface{}) {
	log.Output(2, fmt.Sprint(v...))
}

// Printf calls the standard log.Printf()
func Printf(format string, v ...interface{}) {
	log.Output(2, fmt.Sprintf(format, v...))
}

// Println calls the standard log.Println()
func Println(v ...interface{}) {
	log.Output(2, fmt.Sprintln(v...))
}

// Fatal calls the standard log.Fatal()
func Fatal(v ...interface{}) {
	log.Output(2, fmt.Sprint(v...))
	os.Exit(1)
}

// Fatalf calls the standard log.Fatalf()
func Fatalf(format string, v ...interface{}) {
	log.Output(2, fmt.Sprintf(format, v...))
	os.Exit(1)
}

// Fatalln calls the standard log.Fatalln()
func Fatalln(v ...interface{}) {
	log.Output(2, fmt.Sprintln(v...))
	os.Exit(1)
}

// Debug calls the standard log.Print() with a [DEBUG] prefix
// Debug calls the standard log.Print() with a [DEBUG] prefix
func Debug(v ...interface{}) {
	// No-op in release builds
}

// Debugf calls the standard log.Printf() with a [DEBUG] prefix
func Debugf(format string, v ...interface{}) {
	// No-op in release builds
}
