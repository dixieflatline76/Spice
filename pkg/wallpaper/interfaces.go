package wallpaper

// JobSubmitter abstracts the pipeline submission for testing.
type JobSubmitter interface {
	Submit(job DownloadJob) bool
}
