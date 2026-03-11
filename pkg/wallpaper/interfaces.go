package wallpaper

import "context"

// JobSubmitter abstracts the pipeline submission for testing.
type JobSubmitter interface {
	Submit(ctx context.Context, job DownloadJob) bool
}
