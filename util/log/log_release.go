//go:build release

package log

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dixieflatline76/Spice/v2/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

func init() {
	// Determine the log directory based on the centralized config
	logDir := config.GetAppDir()

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
