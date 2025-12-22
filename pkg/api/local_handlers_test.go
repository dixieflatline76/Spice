package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalHandler_Attribution(t *testing.T) {
	// Setup temporary directory structure
	tempDir := t.TempDir()

	// Create "google_photos" namespace
	gpPath := filepath.Join(tempDir, "google_photos")
	setupCollection(t, gpPath, "gp_col_1", "Vacation", "Alice")

	// Create "other_provider" namespace
	otherPath := filepath.Join(tempDir, "other_provider")
	setupCollection(t, otherPath, "op_col_1", "Vacation", "Bob")

	// Setup Server
	s := NewServer()
	s.RegisterNamespace("google_photos", gpPath)
	s.RegisterNamespace("other_provider", otherPath)
	handler := s.Handler()

	// Test case 1: "google_photos" should suppress author in attribution
	reqGP := httptest.NewRequest("GET", "/local/google_photos/gp_col_1/images?page=1", nil)
	wGP := httptest.NewRecorder()
	handler.ServeHTTP(wGP, reqGP)

	assert.Equal(t, http.StatusOK, wGP.Code)
	var imagesGP []LocalImage
	err := json.Unmarshal(wGP.Body.Bytes(), &imagesGP)
	assert.NoError(t, err)
	assert.NotEmpty(t, imagesGP)
	// Expected: "Vacation" (Author "Alice" suppressed)
	assert.Equal(t, "Vacation", imagesGP[0].Attribution, "Google Photos should suppress author name")

	// Test case 2: "other_provider" should include author
	reqOther := httptest.NewRequest("GET", "/local/other_provider/op_col_1/images?page=1", nil)
	wOther := httptest.NewRecorder()
	handler.ServeHTTP(wOther, reqOther)

	assert.Equal(t, http.StatusOK, wOther.Code)
	var imagesOther []LocalImage
	err = json.Unmarshal(wOther.Body.Bytes(), &imagesOther)
	assert.NoError(t, err)
	assert.NotEmpty(t, imagesOther)
	// Expected: "Vacation (by Bob)"
	assert.Equal(t, "Vacation (by Bob)", imagesOther[0].Attribution, "Standard provider should show full attribution")
}

func setupCollection(t *testing.T, rootPath, colID, desc, author string) {
	colPath := filepath.Join(rootPath, colID)
	err := os.MkdirAll(colPath, 0755)
	assert.NoError(t, err)

	// Create dummy image
	err = os.WriteFile(filepath.Join(colPath, "test.jpg"), []byte("image_data"), 0644)
	assert.NoError(t, err)

	// Create metadata.json
	meta := map[string]interface{}{
		"id":          colID,
		"description": desc,
		"author":      author,
	}
	f, err := os.Create(filepath.Join(colPath, "metadata.json"))
	assert.NoError(t, err)
	defer f.Close()
	err = json.NewEncoder(f).Encode(meta)
	assert.NoError(t, err)
}
