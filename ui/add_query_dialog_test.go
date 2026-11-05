package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addQueryPopupSize opens the Add Query dialog the way the browser extension does on a brand-new
// preferences window (requested while the window content is being built), then sizes/shows the
// window exactly like CreatePreferencesWindow, and returns the resulting popup size.
func addQueryPopupSize(t *testing.T, queue bool) fyne.Size {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	w := app.NewWindow("Spice Preferences")

	sm := NewSettingsManager(w).(*SettingsManager)
	sm.queueDialogs = queue

	// Requested during schema build (provider CreateQueryPanel with a pending extension URL).
	sm.ShowAddQueryDialog(schema.AddQueryConfig{Title: "Add wallhaven Collection"}, "https://wallhaven.cc/search?q=space", "", nil)

	// CreatePreferencesWindow: SetContent -> Resize -> Show -> showQueuedDialogs.
	w.SetContent(newMinSizeContainer(widget.NewLabel("Preferences content")))
	w.Resize(fyne.NewSize(1000, 900))
	w.Show()
	sm.showQueuedDialogs()

	top := w.Canvas().Overlays().Top()
	require.NotNil(t, top, "Add Query dialog must be shown")
	for _, o := range test.WidgetRenderer(top.(fyne.Widget)).Objects() {
		if p, ok := o.(*widget.PopUp); ok {
			return p.Size()
		}
	}
	t.Fatal("Add Query popup not found in overlay")
	return fyne.Size{}
}

func TestAddQueryDialog_NewWindow_ShownAtFullSize(t *testing.T) {
	size := addQueryPopupSize(t, true)
	assert.Equal(t, float32(600), size.Width,
		"Extension-triggered Add Query dialog on a new window must open at its full 600px width")
}

// Documents the Fyne behaviour the queue protects against: a dialog shown before the window has
// content/size is permanently clamped to the tiny pre-Show canvas (the narrow Wallhaven popup bug).
func TestAddQueryDialog_ShownBeforeWindowSized_IsClamped(t *testing.T) {
	size := addQueryPopupSize(t, false)
	assert.Less(t, size.Width, float32(600), "Without queueing, Fyne clamps the dialog (if this fails, Fyne fixed it and the queue may be removable)")
}
