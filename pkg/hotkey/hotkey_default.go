//go:build !darwin

package hotkey

import "golang.design/x/hotkey"

const (
	modCtrl = hotkey.ModCtrl
	modAlt  = hotkey.ModAlt
)
