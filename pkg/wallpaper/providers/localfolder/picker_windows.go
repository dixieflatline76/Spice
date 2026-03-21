//go:build windows

package localfolder

import (
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/util/log"
	"github.com/harry1453/go-common-file-dialog/cfd"
)

func showOSFolderPicker(_ fyne.Window, callback func(string, error)) {
	log.Debugf("[LocalFolder] showOSFolderPicker called (Windows path) - using File Open workaround to show images")
	// Run the Windows Common File Dialog on its own goroutine + OS thread so
	// it never blocks Fyne's main event loop.
	go func() {
		// Pin to one OS thread for COM STA lifetime.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		log.Debugf("[LocalFolder] Creating Windows Open File dialog...")
		config := cfd.DialogConfig{
			Title: i18n.T("Select any image in the desired folder"),
			FileFilters: []cfd.FileFilter{
				{
					DisplayName: i18n.T("Images"),
					Pattern:     "*.jpg;*.jpeg;*.png;*.webp",
				},
			},
		}
		dialog, err := cfd.NewOpenFileDialog(config)
		if err != nil {
			log.Debugf("[LocalFolder] NewOpenFileDialog error: %v", err)
			callback("", err)
			return
		}
		defer func() { _ = dialog.Release() }()

		log.Debugf("[LocalFolder] Showing dialog...")
		file, err := dialog.ShowAndGetResult()
		log.Debugf("[LocalFolder] Dialog result: file=%q, err=%v", file, err)
		if err != nil {
			callback("", err)
			return
		}
		if file == "" {
			callback("", nil)
			return
		}

		// Take the directory of the selected file
		folder := filepath.Dir(file)
		log.Debugf("[LocalFolder] Selected file: %q -> Parent folder: %q", file, folder)
		callback(folder, nil)
	}()
}
