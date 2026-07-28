package wallpaper

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNextDerivativePath(t *testing.T) {
	tests := []struct {
		name        string
		currentPath string
		targetOS    string
		want        string
	}{
		{
			name:        "Windows ignores double buffering",
			currentPath: filepath.Join("some", "dir", "image.jpg"),
			targetOS:    "windows",
			want:        filepath.Join("some", "dir", "image.jpg"),
		},
		{
			name:        "Linux ignores double buffering",
			currentPath: filepath.Join("some", "dir", "image.jpg"),
			targetOS:    "linux",
			want:        filepath.Join("some", "dir", "image.jpg"),
		},
		{
			name:        "macOS first tune appends _A",
			currentPath: filepath.Join("some", "dir", "image.jpg"),
			targetOS:    "darwin",
			want:        filepath.Join("some", "dir", "image_A.jpg"),
		},
		{
			name:        "macOS transitions from _A to _B",
			currentPath: filepath.Join("some", "dir", "image_A.jpg"),
			targetOS:    "darwin",
			want:        filepath.Join("some", "dir", "image_B.jpg"),
		},
		{
			name:        "macOS transitions from _B to _A",
			currentPath: filepath.Join("some", "dir", "image_B.png"),
			targetOS:    "darwin",
			want:        filepath.Join("some", "dir", "image_A.png"),
		},
		{
			name:        "macOS handles files with no extension",
			currentPath: filepath.Join("some", "dir", "image_A"),
			targetOS:    "darwin",
			want:        filepath.Join("some", "dir", "image_B"),
		},
		{
			name:        "macOS handles complex paths",
			currentPath: filepath.Join("User", "Downloads", "my.picture_A.jpeg"),
			targetOS:    "darwin",
			want:        filepath.Join("User", "Downloads", "my.picture_B.jpeg"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextDerivativePath(tt.currentPath, tt.targetOS)
			assert.Equal(t, tt.want, got)
		})
	}
}
