//go:build !windows && !darwin

package localfolder

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

func showOSFolderPicker(parent fyne.Window, callback func(string, error)) {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			callback("", err)
			return
		}
		if uri == nil {
			callback("", nil)
			return
		}
		callback(uri.Path(), nil)
	}, parent)
}
