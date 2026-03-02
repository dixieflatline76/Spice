package log

import (
	"fmt"
	"log"
)

// Debug logging uses function pointers so that the implementation can be swapped
// at runtime between a no-op and the real log output.
// - Dev builds (!release): debug is ON by default (init in log.go calls SetDebugEnabled(true))
// - Release builds: debug is OFF by default (no-op function pointer)
// Users can toggle debug logging at runtime via Preferences.

// noOpDebug is a no-op implementation for when debug logging is disabled.
var noOpDebug = func(_ ...interface{}) {}

// noOpDebugf is a no-op implementation for when debug logging is disabled.
var noOpDebugf = func(_ string, _ ...interface{}) {}

// realDebug is the implementation that actually writes to the log output.
// Call depth is 3 because: caller -> Debug() -> realDebug() -> log.Output()
var realDebug = func(v ...interface{}) {
	_ = log.Output(3, "[DEBUG] "+fmt.Sprint(v...))
}

// realDebugf is the implementation that actually writes to the log output.
// Call depth is 3 because: caller -> Debugf() -> realDebugf() -> log.Output()
var realDebugf = func(format string, v ...interface{}) {
	_ = log.Output(3, fmt.Sprintf("[DEBUG] "+format, v...))
}

// Active function pointers — default to no-op (release behavior).
var debugFn = noOpDebug
var debugfFn = noOpDebugf
var debugEnabled bool

// SetDebugEnabled swaps the debug function pointers between no-op and real output.
func SetDebugEnabled(enabled bool) {
	debugEnabled = enabled
	if enabled {
		debugFn = realDebug
		debugfFn = realDebugf
	} else {
		debugFn = noOpDebug
		debugfFn = noOpDebugf
	}
}

// IsDebugEnabled returns whether debug logging is currently active.
func IsDebugEnabled() bool {
	return debugEnabled
}

// Debug logs with a [DEBUG] prefix if debug mode is enabled.
func Debug(v ...interface{}) {
	debugFn(v...)
}

// Debugf logs with a [DEBUG] prefix if debug mode is enabled.
func Debugf(format string, v ...interface{}) {
	debugfFn(format, v...)
}
