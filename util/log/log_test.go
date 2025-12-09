package log

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestLogging(t *testing.T) {
	// Capture standard log output
	var buf bytes.Buffer
	log.SetOutput(&buf)

	// Since we can't easily undo the init() side effects on the real log file,
	// we just test that our wrapper functions write to the standard logger.

	tests := []struct {
		name     string
		fn       func()
		expected string
	}{
		{
			name: "Print",
			fn: func() {
				Print("test print")
			},
			expected: "test print",
		},
		{
			name: "Printf",
			fn: func() {
				Printf("test printf %d", 123)
			},
			expected: "test printf 123",
		},
		{
			name: "Println",
			fn: func() {
				Println("test println")
			},
			expected: "test println",
		},
		{
			name: "Debug",
			fn: func() {
				Debug("test debug")
			},
			expected: "[DEBUG] test debug",
		},
		{
			name: "Debugf",
			fn: func() {
				Debugf("test debugf %s", "foo")
			},
			expected: "[DEBUG] test debugf foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.fn()
			if !strings.Contains(buf.String(), tt.expected) {
				t.Errorf("Expected log to contain %q, but got %q", tt.expected, buf.String())
			}
		})
	}
}

// NOTE: Testing Fatal* functions requires a subprocess, which is often overkill for simple wrappers.
