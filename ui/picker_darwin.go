//go:build darwin

package ui

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

func showOSFolderPicker(_ fyne.Window, callback func(string, error)) {
	log.Debugf("[LocalFolder] showOSFolderPicker called (macOS path) - using osascript")

	// Run in a separate goroutine to avoid blocking the main Fyne event loop
	go func() {
		prompt := i18n.T("Select Folder")
		script := fmt.Sprintf(`POSIX path of (choose folder with prompt "%s")`, prompt)
		cmd := exec.Command("osascript", "-e", script)

		var outbuf, errbuf bytes.Buffer
		cmd.Stdout = &outbuf
		cmd.Stderr = &errbuf

		log.Debugf("[LocalFolder] Executing osascript dialog...")
		err := cmd.Run()

		if err != nil {
			stderrStr := errbuf.String()
			log.Debugf("[LocalFolder] osascript returned error: %v, stderr: %q", err, stderrStr)

			// AppleScript returns error -128 when the user clicks Cancel.
			if strings.Contains(stderrStr, "User canceled") || strings.Contains(stderrStr, "-128") {
				log.Debugf("[LocalFolder] User cancelled the macOS folder dialog.")
				callback("", nil)
				return
			}

			callback("", fmt.Errorf("osascript error: %w, stderr: %s", err, stderrStr))
			return
		}

		folderPath := strings.TrimSpace(outbuf.String())
		log.Debugf("[LocalFolder] Selected macOS folder: %q", folderPath)
		callback(folderPath, nil)
	}()
}

func showOSAppPicker(_ fyne.Window, callback func(string, error)) {
	go func() {
		prompt := i18n.T("Select Application or Game")
		script := fmt.Sprintf(`POSIX path of (choose file of type {"app", "public.executable"} with prompt "%s" default location (path to applications folder))`, prompt)
		cmd := exec.Command("osascript", "-e", script)

		var outbuf, errbuf bytes.Buffer
		cmd.Stdout = &outbuf
		cmd.Stderr = &errbuf

		if err := cmd.Run(); err != nil {
			stderrStr := errbuf.String()
			if strings.Contains(stderrStr, "User canceled") || strings.Contains(stderrStr, "-128") {
				callback("", nil)
				return
			}
			callback("", fmt.Errorf("osascript error: %w, stderr: %s", err, stderrStr))
			return
		}

		appPath := strings.TrimRight(strings.TrimSpace(outbuf.String()), "/")
		if appPath == "" {
			callback("", nil)
			return
		}
		callback(filepath.Base(appPath), nil)
	}()
}
