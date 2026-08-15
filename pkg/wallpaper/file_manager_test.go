package wallpaper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestCleanupOrphans_DeepResolutionStructure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spice-cleanup-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	fm := NewFileManager(tempDir)
	err = fm.EnsureDirs()
	assert.NoError(t, err)

	// 1. Create a deep resolution folder that mimicks the new structure
	// Structure: fitted/quality/standard/3440x1440/

	deepRelPath := filepath.Join(FittedRootDir, QualityDir, StandardDir, "3440x1440")
	fullDeepPath := filepath.Join(tempDir, deepRelPath)
	err = os.MkdirAll(fullDeepPath, 0755)
	assert.NoError(t, err)

	// 2. Put an orphan file there
	orphanID := "orphan_image"
	orphanPath := filepath.Join(fullDeepPath, orphanID+".jpg")
	err = os.WriteFile(orphanPath, []byte("fake image data"), 0644)
	assert.NoError(t, err)

	// 3. Put a valid file there
	validID := "valid_image"
	validPath := filepath.Join(fullDeepPath, validID+".jpg")
	err = os.WriteFile(validPath, []byte("fake valid data"), 0644)
	assert.NoError(t, err)

	// 4. Run Cleanup
	known := map[string]bool{
		validID: true,
	}
	fm.CleanupOrphans(known)

	// 5. Verify Orphan is gone
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Errorf("Orphan file should have been deleted: %s", orphanPath)
	}

	// 6. Verify Valid file is kept
	if _, err := os.Stat(validPath); os.IsNotExist(err) {
		t.Errorf("Valid file should have been kept: %s", validPath)
	}
}
func TestDeepDeleteBatch(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	if err := fm.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	ids := []string{"id1", "id2"}
	paths := []string{}

	for _, id := range ids {
		master, _ := fm.GetMasterPath(id, ".jpg")
		if err := os.WriteFile(master, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, master)

		deriv, _ := fm.GetDerivativePath(id, ".jpg", filepath.Join(FittedRootDir, QualityDir, StandardDir))
		if err := os.WriteFile(deriv, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, deriv)
	}

	// Action
	if err := fm.DeepDeleteBatch(ids); err != nil {
		t.Fatalf("DeepDeleteBatch failed: %v", err)
	}

	// Verify
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("File %s should have been deleted", p)
		}
	}
}

func TestFileManager_AsyncAndDrain(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	if err := fm.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	id := "async_test_img"
	master, _ := fm.GetMasterPath(id, ".jpg")
	if err := os.WriteFile(master, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	deriv, _ := fm.GetDerivativePath(id, ".jpg", filepath.Join(FittedRootDir, QualityDir, StandardDir))
	if err := os.WriteFile(deriv, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Launch async deletion
	fm.AsyncDeepDelete(id)

	// Drain
	drained := fm.WaitTimeout(2 * time.Second)
	assert.True(t, drained, "WaitTimeout should return true when operations complete")

	// Verify deleted
	_, err := os.Stat(master)
	assert.True(t, os.IsNotExist(err), "Master should be deleted")
	_, err = os.Stat(deriv)
	assert.True(t, os.IsNotExist(err), "Derivative should be deleted")
}

func TestFileManager_Close(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	if err := fm.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	fm.Close()
	assert.True(t, fm.closing.Load())

	// Async calls after close should be no-ops
	fm.AsyncDeepDelete("some_id")
	fm.AsyncCleanupOrphans(map[string]bool{})
}

func TestFileManager_SpiceTmpCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	if err := fm.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	derivDir := filepath.Join(tmpDir, FittedRootDir, QualityDir, StandardDir)

	// Create master and derivatives including .spice_tmp
	masterPath, err := fm.GetMasterPath("art_101", ".jpg")
	assert.NoError(t, err)
	_ = os.WriteFile(masterPath, []byte("master"), 0644)

	derivPath := filepath.Join(derivDir, "art_101.jpg")
	_ = os.WriteFile(derivPath, []byte("deriv"), 0644)

	tmpDerivPath := filepath.Join(derivDir, "art_101.jpg.spice_tmp")
	_ = os.WriteFile(tmpDerivPath, []byte("tmp_deriv"), 0644)

	// Verify files exist
	assert.FileExists(t, masterPath)
	assert.FileExists(t, derivPath)
	assert.FileExists(t, tmpDerivPath)

	// Test 1: DeepDeleteBatch deletes master, normal derivative, and .spice_tmp derivative
	err = fm.DeepDeleteBatch([]string{"art_101"})
	assert.NoError(t, err)

	assert.NoFileExists(t, masterPath)
	assert.NoFileExists(t, derivPath)
	assert.NoFileExists(t, tmpDerivPath)

	// Test 2: CleanupOrphans cleans orphaned .spice_tmp derivative
	orphanDeriv := filepath.Join(derivDir, "orphan_202.jpg")
	_ = os.WriteFile(orphanDeriv, []byte("orphan"), 0644)
	orphanTmpDeriv := filepath.Join(derivDir, "orphan_202.jpg.spice_tmp")
	_ = os.WriteFile(orphanTmpDeriv, []byte("orphan_tmp"), 0644)

	assert.FileExists(t, orphanDeriv)
	assert.FileExists(t, orphanTmpDeriv)

	fm.CleanupOrphans(map[string]bool{"art_101": true}) // orphan_202 not known

	assert.NoFileExists(t, orphanDeriv)
	assert.NoFileExists(t, orphanTmpDeriv)
}
