package wallpaper

import (
	"context"
	"image"
)

// JobSubmitter abstracts the pipeline submission for testing.
type JobSubmitter interface {
	Submit(ctx context.Context, job DownloadJob) bool
}

// TrayCapableOS is an optional OS extension for platforms where the tray icon
// appears on every display's menu bar (macOS with "Displays have separate Spaces").
// OS implementations that do not implement it get the single-tray layout.
type TrayCapableOS interface {
	HasPerScreenTray() bool                 // Must be cheap; called on every tick and click.
	GetCursorPosition() (image.Point, bool) // Same units as Monitor.Bounds. Must be thread-safe.
}
