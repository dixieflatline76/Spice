package api

import (
	"encoding/json"
	"fmt"
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

	// Create dummy entries
	// G306: Expect WriteFile permissions to be 0600
	err = os.WriteFile(filepath.Join(colPath, "image1.jpg"), []byte("fake image content"), 0600)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(colPath, "image2.png"), []byte("fake image content"), 0600)
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

func TestLocalHandler_Security_Direct(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	gpPath := filepath.Join(tempDir, "google_photos")
	setupCollection(t, gpPath, "safe_col", "Safe", "Alice")
	s := NewServer()
	s.RegisterNamespace("google_photos", gpPath)

	tests := []struct {
		name       string
		path       string // Raw path to simulate bypass of ServerMux cleaning or RawPath usage
		wantStatus int
	}{
		{
			name:       "Valid Asset",
			path:       "/local/google_photos/safe_col/assets/image1.jpg",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Path Traversal to Root",
			path:       "/local/google_photos/../safe_col/assets/image1.jpg",
			wantStatus: http.StatusBadRequest, // Blocked by collectionID strict check
		},
		{
			name:       "Collection Traversal",
			path:       "/local/google_photos/../gp_col_out/assets/test.jpg",
			wantStatus: http.StatusBadRequest, // Blocked by collectionID strict check
		},
		{
			name:       "Filename Traversal",
			path:       "/local/google_photos/safe_col/assets/../metadata.json",
			wantStatus: http.StatusBadRequest, // Blocked by specialized filename check in handleLocalAsset
		},
		{
			name:       "Filename with Windows Separator",
			path:       "/local/google_photos/safe_col/assets/foo\\bar.jpg",
			wantStatus: http.StatusBadRequest, // Blocked by filename check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			// Manually set RequestURI/URL.Path to ensure it's not cleaned by NewRequest if possible,
			// though NewRequest parses it.
			// We can manually invoke handleLocal which uses r.URL.Path
			// For specific ".." testing, we might need to rely on the fact we are passing it directly to split.

			w := httptest.NewRecorder()
			// Direct call to bypass ServeMux cleaning for testing handler resilience
			s.handleLocal(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("path %q: got status %d, want %d Body: %s", tt.path, w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestLocalHandler_Pagination(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	gpPath := filepath.Join(tempDir, "listing_test")
	colPath := filepath.Join(gpPath, "many_images")
	err := os.MkdirAll(colPath, 0755)
	assert.NoError(t, err)

	// Create 10 images: img0.jpg -> img9.jpg
	for i := 0; i < 10; i++ {
		fname := fmt.Sprintf("img%d.jpg", i)
		err = os.WriteFile(filepath.Join(colPath, fname), []byte("fake"), 0600)
		assert.NoError(t, err)
	}

	s := NewServer()
	s.RegisterNamespace("listing_test", gpPath)
	handler := s.Handler()

	// Page 1, PerPage 4 -> should get img0, img1, img2, img3
	req1 := httptest.NewRequest("GET", "/local/listing_test/many_images/images?page=1&per_page=4", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	var images1 []LocalImage
	err = json.Unmarshal(w1.Body.Bytes(), &images1)
	assert.NoError(t, err)
	assert.Len(t, images1, 4)
	assert.Equal(t, "img0", images1[0].ID)
	assert.Equal(t, "img3", images1[3].ID)

	// Page 2, PerPage 4 -> should get img4, img5, img6, img7
	req2 := httptest.NewRequest("GET", "/local/listing_test/many_images/images?page=2&per_page=4", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var images2 []LocalImage
	err = json.Unmarshal(w2.Body.Bytes(), &images2)
	assert.NoError(t, err)
	assert.Len(t, images2, 4)
	assert.Equal(t, "img4", images2[0].ID)

	// Page 3, PerPage 4 -> should get img8, img9 (only 2 left)
	req3 := httptest.NewRequest("GET", "/local/listing_test/many_images/images?page=3&per_page=4", nil)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	var images3 []LocalImage
	err = json.Unmarshal(w3.Body.Bytes(), &images3)
	assert.NoError(t, err)
	assert.Len(t, images3, 2)
	assert.Equal(t, "img8", images3[0].ID)
	assert.Equal(t, "img9", images3[1].ID)

	// Page 4 -> Empty
	req4 := httptest.NewRequest("GET", "/local/listing_test/many_images/images?page=4&per_page=4", nil)
	w4 := httptest.NewRecorder()
	handler.ServeHTTP(w4, req4)

	assert.Equal(t, http.StatusOK, w4.Code)
	var images4 []LocalImage
	err = json.Unmarshal(w4.Body.Bytes(), &images4)
	assert.NoError(t, err)
	assert.Len(t, images4, 0)
}
