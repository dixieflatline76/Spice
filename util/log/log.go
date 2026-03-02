//go:build !release

package log

import (
	"fmt"
	"io"
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

	fileLogger := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10, // MB
		MaxBackups: 2,
		MaxAge:     28, // days
		Compress:   true,
	}

	// Write to both stdout and file
	// Wrap os.Stdout in a safe writer that ignores errors (e.g. in GUI mode)
	safeStdout := &safeWriter{w: os.Stdout}
	log.SetOutput(io.MultiWriter(safeStdout, fileLogger))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Dev builds: enable debug logging by default
	SetDebugEnabled(true)
}

// safeWriter wraps an io.Writer and ignores errors, returning success.
// This is useful when writing to stdout in a Windows GUI app where it might fail.
type safeWriter struct {
	w io.Writer
}

func (sw *safeWriter) Write(p []byte) (int, error) {
	_, _ = sw.w.Write(p)
	// Always return success to allow MultiWriter to continue to the next writer
	return len(p), nil
}

// Print calls the standard log.Print()
func Print(v ...interface{}) {
	_ = log.Output(2, fmt.Sprint(v...))
}

// Printf calls the standard log.Printf()
func Printf(format string, v ...interface{}) {
	_ = log.Output(2, fmt.Sprintf(format, v...))
}

// Println calls the standard log.Println()
func Println(v ...interface{}) {
	_ = log.Output(2, fmt.Sprintln(v...))
}

// Fatal calls the standard log.Fatal()
func Fatal(v ...interface{}) {
	_ = log.Output(2, fmt.Sprint(v...))
	os.Exit(1)
}

// Fatalf calls the standard log.Fatalf()
func Fatalf(format string, v ...interface{}) {
	_ = log.Output(2, fmt.Sprintf(format, v...))
	os.Exit(1)
}

// Fatalln calls the standard log.Fatalln()
func Fatalln(v ...interface{}) {
	_ = log.Output(2, fmt.Sprintln(v...))
	os.Exit(1)
}
