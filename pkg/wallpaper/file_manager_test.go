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
	masterPath, _ := fm.GetMasterPath(id, ".jpg")
	if err := os.WriteFile(masterPath, []byte("master"), 0644); err != nil {
		t.Fatalf("Failed to create master: %v", err)
	}

	// Scenario 1: New Setup
	// Use joined constants for new nested paths
	fittedPath, _ := fm.GetDerivativePath(id, ".jpg", filepath.Join(FittedRootDir, QualityDir, StandardDir))
	if err := os.WriteFile(fittedPath, []byte("fitted"), 0644); err != nil {
		t.Fatalf("Failed to create fitted: %v", err)
	}

	// Create a "Flexibility FaceCrop" file (to test Deep Clean functionality across branches)
	faceCropPath, _ := fm.GetDerivativePath(id, ".jpg", filepath.Join(FittedRootDir, FlexibilityDir, FaceCropDir))
	if err := os.WriteFile(faceCropPath, []byte("facecrop"), 0644); err != nil {
		t.Fatalf("Failed to create facecrop: %v", err)
	}

	// Verify creation
	if _, err := os.Stat(masterPath); os.IsNotExist(err) {
		t.Fatal("Master not created")
	}

	// 2. Execute Deep Delete
	if err := fm.DeepDelete(id); err != nil {
		t.Fatalf("DeepDelete failed: %v", err)
	}

	// 3. Verify
	if _, err := os.Stat(masterPath); !os.IsNotExist(err) {
		t.Errorf("Master should be deleted: %s", masterPath)
	}
	if _, err := os.Stat(fittedPath); !os.IsNotExist(err) {
		t.Errorf("Fitted should be deleted: %s", fittedPath)
	}
	if _, err := os.Stat(faceCropPath); !os.IsNotExist(err) {
		t.Errorf("FaceCrop (Flexible) should be deleted: %s", faceCropPath)
	}
}

func TestResolvePaths(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)

	master, err := fm.GetMasterPath("id", ".jpg")
	if err != nil {
		t.Fatalf("GetMasterPath failed: %v", err)
	}
	expectedMaster := filepath.Join(tmpDir, "id.jpg")
	if master != expectedMaster {
		t.Errorf("Expected master %s, got %s", expectedMaster, master)
	}

	deriv, err := fm.GetDerivativePath("id", ".jpg", "subdir")
	if err != nil {
		t.Fatalf("GetDerivativePath failed: %v", err)
	}
	expectedDeriv := filepath.Join(tmpDir, "subdir", "id.jpg")
	if deriv != expectedDeriv {
		t.Errorf("Expected derivative %s, got %s", expectedDeriv, deriv)
	}
}

func TestSecurityValidation(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)

	// Case 1: Path Traversal
	if _, err := fm.GetMasterPath("../../etc/passwd", ".jpg"); err == nil {
		t.Error("Expected error for path traversal ID, got nil")
	}

	// Case 2: Extension Traversal (Should be failed by string check or caller)
	if _, err := fm.GetMasterPath("safe", "../.jpg"); err == nil {
		t.Error("Expected error for path traversal Extension, got nil")
	}

	// Case 3: Derivative Traversal
	if _, err := fm.GetDerivativePath("safe", ".jpg", "../../../root"); err == nil {
		t.Error("Expected error for path traversal DerivativeType, got nil")
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

	// 1. Create "valid" files
	validMaster := filepath.Join(tmpDir, "valid.jpg")
	if err := os.WriteFile(validMaster, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	validDeriv, _ := fm.GetDerivativePath("valid", ".jpg", filepath.Join(FittedRootDir, QualityDir, StandardDir))
	if err := os.WriteFile(validDeriv, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create "orphan" files
	orphanMaster := filepath.Join(tmpDir, "orphan.png")
	if err := os.WriteFile(orphanMaster, []byte("trash"), 0644); err != nil {
		t.Fatal(err)
	}

	orphanDeriv, _ := fm.GetDerivativePath("orphan", ".png", filepath.Join(FittedRootDir, FlexibilityDir, FaceBoostDir))
	if err := os.WriteFile(orphanDeriv, []byte("trash"), 0644); err != nil {
		t.Fatal(err)
	}

	// Known Set
	known := map[string]bool{"valid": true}

	// Run Cleanup
	fm.CleanupOrphans(known)

	// Verify Valid Remain
	if _, err := os.Stat(validMaster); os.IsNotExist(err) {
		t.Error("Valid master deleted incorrectly")
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
