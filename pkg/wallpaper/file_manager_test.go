package wallpaper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeepDelete(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	if err := fm.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	id := "test_img"

	// Create Master
	masterPath := fm.GetMasterPath(id, ".jpg")
	if err := os.WriteFile(masterPath, []byte("master"), 0644); err != nil {
		t.Fatalf("Failed to create master: %v", err)
	}

	// Create Derivatives
	fittedPath := fm.GetDerivativePath(id, ".jpg", FittedImgDir)
	if err := os.WriteFile(fittedPath, []byte("fitted"), 0644); err != nil {
		t.Fatalf("Failed to create fitted: %v", err)
	}

	faceCropPath := fm.GetDerivativePath(id, ".jpg", FittedFaceCropImgDir)
	if err := os.WriteFile(faceCropPath, []byte("facecrop"), 0644); err != nil {
		t.Fatalf("Failed to create facecrop: %v", err)
	}

	// Verify creation
	if _, err := os.Stat(masterPath); os.IsNotExist(err) {
		t.Fatal("Master not created")
	}

	// Delete
	if err := fm.DeepDelete(id); err != nil {
		t.Errorf("DeepDelete failed: %v", err)
	}

	// Verify Deletion
	if _, err := os.Stat(masterPath); !os.IsNotExist(err) {
		t.Errorf("Master should be deleted: %s", masterPath)
	}
	if _, err := os.Stat(fittedPath); !os.IsNotExist(err) {
		t.Errorf("Fitted should be deleted: %s", fittedPath)
	}
	if _, err := os.Stat(faceCropPath); !os.IsNotExist(err) {
		t.Errorf("FaceCrop should be deleted: %s", faceCropPath)
	}
}

func TestResolvePaths(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)

	master := fm.GetMasterPath("id", ".jpg")
	expectedMaster := filepath.Join(tmpDir, "id.jpg")
	if master != expectedMaster {
		t.Errorf("Expected master %s, got %s", expectedMaster, master)
	}

	deriv := fm.GetDerivativePath("id", ".jpg", "subdir")
	expectedDeriv := filepath.Join(tmpDir, "subdir", "id.jpg")
	if deriv != expectedDeriv {
		t.Errorf("Expected derivative %s, got %s", expectedDeriv, deriv)
	}
}

func TestCleanupOrphans(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	if err := fm.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	// Setup:
	// "valid": Known ID. Should remain.
	// "orphan": Unknown ID. Should be deleted.

	// Create Valid Files
	validMaster := fm.GetMasterPath("valid", ".jpg")
	validDeriv := fm.GetDerivativePath("valid", ".jpg", FittedImgDir)
	_ = os.WriteFile(validMaster, []byte("keep"), 0644)
	_ = os.WriteFile(validDeriv, []byte("keep"), 0644)

	// Create Orphan Files
	orphanMaster := fm.GetMasterPath("orphan", ".png")
	orphanDeriv := fm.GetDerivativePath("orphan", ".png", FittedFaceCropImgDir)
	_ = os.WriteFile(orphanMaster, []byte("delete"), 0644)
	_ = os.WriteFile(orphanDeriv, []byte("delete"), 0644)

	// Known Set
	known := map[string]bool{"valid": true}

	// Run Cleanup
	fm.CleanupOrphans(known)

	// Verify Valid Remain
	if _, err := os.Stat(validMaster); os.IsNotExist(err) {
		t.Error("Valid master delete incorrectly")
	}
	if _, err := os.Stat(validDeriv); os.IsNotExist(err) {
		t.Error("Valid derivative deleted incorrectly")
	}

	// Verify Orphans Deleted
	if _, err := os.Stat(orphanMaster); !os.IsNotExist(err) {
		t.Error("Orphan master NOT deleted")
	}
	if _, err := os.Stat(orphanDeriv); !os.IsNotExist(err) {
		t.Error("Orphan derivative NOT deleted")
	}
}
