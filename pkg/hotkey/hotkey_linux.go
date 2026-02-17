//go:build linux

package hotkey

import (
	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/util/log"
)

// StartListeners is a stub for Linux where hotkeys are not yet supported/required.
func StartListeners(mgr ui.PluginManager) {
	log.Println("Global hotkeys are currently disabled on Linux.")
}
