package googlephotos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSaveInitialMetadata(t *testing.T) {
	guid := "test-guid-123"
	// Note: Provider uses hardcoded os.TempDir()/spice/google_photos
	basePath := filepath.Join(os.TempDir(), "spice", "google_photos")
	targetDir := filepath.Join(basePath, guid)

	// Ensure cleanup
	defer os.RemoveAll(targetDir)

	// Create dir manually as saveInitialMetadata expects it
	err := os.MkdirAll(targetDir, 0755)
	assert.NoError(t, err)

	p := &Provider{}

	links := map[string]string{
		"img1.jpg": "http://google.com/img1",
	}

	err = p.saveInitialMetadata(guid, links)
	assert.NoError(t, err)

	// Read back
	data, err := os.ReadFile(filepath.Join(targetDir, "metadata.json"))
	assert.NoError(t, err)

	var meta map[string]interface{}
	err = json.Unmarshal(data, &meta)
	assert.NoError(t, err)

	assert.Equal(t, guid, meta["id"])
	assert.Equal(t, "", meta["author"])

	files, ok := meta["files"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "http://google.com/img1", files["img1.jpg"])
}

func TestUpdateMetadata_PreservesFiles(t *testing.T) {
	guid := "test-guid-456"
	basePath := filepath.Join(os.TempDir(), "spice", "google_photos")
	targetDir := filepath.Join(basePath, guid)

	defer os.RemoveAll(targetDir)
	err := os.MkdirAll(targetDir, 0755)
	assert.NoError(t, err)

	p := &Provider{}

	// 1. Initial Save
	links := map[string]string{"foo.jpg": "bar_url"}
	err = p.saveInitialMetadata(guid, links)
	assert.NoError(t, err)

	// 2. Update Description
	err = p.updateMetadata(guid, "New Description")
	assert.NoError(t, err)

	// 3. Verify
	data, _ := os.ReadFile(filepath.Join(targetDir, "metadata.json"))
	var meta map[string]interface{}
	err = json.Unmarshal(data, &meta)
	assert.NoError(t, err)

	assert.Equal(t, "New Description", meta["description"])

	// Files map should still exist and match
	files, ok := meta["files"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "bar_url", files["foo.jpg"])
}
