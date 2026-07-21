package wallpaper

import (
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
)

func newHealthyImage(t *testing.T, fm *FileManager, id, queryID, resolution string, flags map[string]bool) provider.Image {
	t.Helper()
	createMasterFile(t, fm, id)
	derivPath := filepath.Join(fm.rootDir, "fitted", id+"_"+resolution+".jpg")
	return provider.Image{
		ID:              id,
		SourceQueryID:   queryID,
		ProcessingFlags: copyFlags(flags),
		DerivativePaths: map[string]string{resolution: derivPath},
	}
}
