//go:build windows
// +build windows

package sysinfo

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
	getDpiForSystem  = user32.NewProc("GetDpiForSystem")
)

const (
	// SMCXScreen is the index for the screen width system metric.
	SMCXScreen = 0
	// SMCYScreen is the index for the screen height system metric.
	SMCYScreen = 1
	// smRemoteSession is the index for the remote session system metric.
	smRemoteSession = 0x1000
)

// GetScreenDimensions returns the primary desktop dimension (width and height) in pixels.
func GetScreenDimensions() (int, int, error) {
	var width, height uintptr
	var err error

	width, _, err = getSystemMetrics.Call(uintptr(SMCXScreen))
	if err != windows.NOERROR {
		return 0, 0, err
	}
	height, _, err = getSystemMetrics.Call(uintptr(SMCYScreen))
	if err != windows.NOERROR {
		return 0, 0, err
	}

	return int(width), int(height), nil
}

// GetOSDisplayScale returns the OS-level UI scaling factor (e.g. 1.0 for 100%, 1.75 for 175%).
// It safely falls back to 1.0 on older systems or error.
func GetOSDisplayScale() float32 {
	if err := getDpiForSystem.Find(); err != nil {
		return 1.0
	}

	dpi, _, _ := getDpiForSystem.Call()
	if dpi > 0 {
		return float32(dpi) / 96.0
	}
	return 1.0
}

// IsRemoteSession returns true when the process is running inside a native
// Windows Remote Desktop (RDP) session. This uses the official Windows API
// which is only set for Microsoft's own RDP — third-party tools like Chrome
// Remote Desktop, TeamViewer, etc. do not trigger this flag.
func IsRemoteSession() bool {
	ret, _, _ := getSystemMetrics.Call(uintptr(smRemoteSession))
	return ret != 0
}

// CanCreateWindows probes whether the system can create OpenGL rendering contexts.
// Because Fyne's GLFW implementation hard-codes an os.Exit(1) if it fails to create
// an OpenGL window, we cannot simply use recover() in the main app.
// Instead, we spawn ourselves as a subprocess with a special flag. If the subprocess
// exits with 0, OpenGL works. If it exits with 1, OpenGL failed.
// We run this dynamically every time to catch state changes (e.g. RDP reconnects).
func CanCreateWindows() bool {
	return probeOpenGLSubprocess()
}

func probeOpenGLSubprocess() bool {

	exe, err := os.Executable()
	if err != nil {
		return false // fallback if we can't find ourselves
	}

	cmd := exec.Command(exe, "-probe-gl")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	err = cmd.Run()
	if err != nil {
		// Exit status 1 means Fyne's initFailed triggered os.Exit(1)
		return false
	}

	return true
}
