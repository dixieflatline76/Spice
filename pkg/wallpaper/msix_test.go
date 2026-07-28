//go:build windows
// +build windows

package wallpaper

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dixieflatline76/Spice/v2/config"
)

func TestMSIXStagingPaths(t *testing.T) {
	// Setup temp directories for testing staging
	tmpDir, err := os.MkdirTemp("", "spice_msix_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	picStaging := filepath.Join(tmpDir, "Pictures", "SpiceWallpapers")
	docStaging := filepath.Join(tmpDir, "Documents", "Spice")

	// Save original vars and restore after test
	origFamily := msixPackageFamilyName
	origPicStaging := msixPictureStaging
	origDocStaging := msixDocumentStaging

	defer func() {
		msixPackageFamilyName = origFamily
		msixPictureStaging = origPicStaging
		msixDocumentStaging = origDocStaging
	}()

	t.Run("Non-MSIX environment returns original paths unchanged", func(t *testing.T) {
		msixPackageFamilyName = ""
		msixPictureStaging = ""
		msixDocumentStaging = ""

		samplePath := filepath.Join(tmpDir, "sample.jpg")
		assert.Equal(t, samplePath, resolveMSIXPicturePath(samplePath))
		assert.Equal(t, samplePath, resolveMSIXDocumentPath(samplePath))
	})

	t.Run("MSIX environment stages pictures and documents to distinct locations preserving relative structure", func(t *testing.T) {
		msixPackageFamilyName = "TestPackage_123"
		msixPictureStaging = picStaging
		msixDocumentStaging = docStaging

		// Create source file inside working dir
		workingDir := config.GetWorkingDir()
		require.NoError(t, os.MkdirAll(filepath.Join(workingDir, "cache", "metmuseum"), 0755))
		
		srcPic := filepath.Join(workingDir, "sample_wallpaper.jpg")
		srcDoc := filepath.Join(workingDir, "cache", "metmuseum", "american_art.html")

		require.NoError(t, os.WriteFile(srcPic, []byte("fake image data"), 0600))
		require.NoError(t, os.WriteFile(srcDoc, []byte("<html>fake gallery</html>"), 0600))
		defer os.Remove(srcPic)
		defer os.Remove(srcDoc)

		// Test Picture Staging
		stagedPic := resolveMSIXPicturePath(srcPic)
		expectedPic := filepath.Join(picStaging, "sample_wallpaper.jpg")
		assert.Equal(t, expectedPic, stagedPic)
		assert.FileExists(t, stagedPic)

		// Test Document Staging with subfolder structure
		stagedDoc := resolveMSIXDocumentPath(srcDoc)
		expectedDoc := filepath.Join(docStaging, "cache", "metmuseum", "american_art.html")
		assert.Equal(t, expectedDoc, stagedDoc)
		assert.FileExists(t, stagedDoc)
	})
}
