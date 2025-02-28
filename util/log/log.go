//go:build !release

package log

import "log"

// Print calls the standard log.Print()
func Print(v ...interface{}) {
	log.Print(v...)
}

// Printf calls the standard log.Printf()
func Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// Println calls the standard log.Println()
func Println(v ...interface{}) {
	log.Println(v...)
}

// Fatal calls the standard log.Fatal()
func Fatal(v ...interface{}) {
	log.Fatal(v...)
}

// Fatalf calls the standard log.Fatalf()
func Fatalf(format string, v ...interface{}) {
	log.Fatalf(format, v...)
}

// Fatalln calls the standard log.Fatalln()
func Fatalln(v ...interface{}) {
	log.Fatalln(v...)
}
