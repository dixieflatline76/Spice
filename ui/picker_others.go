//go:build !windows && !darwin

package ui

import (
	"path/filepath"

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

func showOSAppPicker(parent fyne.Window, callback func(string, error)) {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			callback("", err)
			return
		}
		_ = reader.Close()
		callback(filepath.Base(reader.URI().Path()), nil)
	}, parent)
}
