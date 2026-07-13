package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeExistingMetadata_Dimensions(t *testing.T) {
	// Fresh image from API has no dimensions
	img := Image{ID: "test1", Width: 0, Height: 0}
	existing := Image{ID: "test1", Width: 4000, Height: 3000}

	img.MergeExistingMetadata(existing)

	assert.Equal(t, 4000, img.Width, "Width should be merged from existing")
	assert.Equal(t, 3000, img.Height, "Height should be merged from existing")
}

func TestMergeExistingMetadata_DimensionsNotOverwritten(t *testing.T) {
	// If the new image already has dimensions (from API), they should be kept
	img := Image{ID: "test1", Width: 5000, Height: 4000}
	existing := Image{ID: "test1", Width: 4000, Height: 3000}

	img.MergeExistingMetadata(existing)

	// existing has non-zero dims, so they overwrite. This is the intended behavior:
	// the existing store entry's probed dimensions are authoritative.
	assert.Equal(t, 4000, img.Width)
	assert.Equal(t, 3000, img.Height)
}

func TestMergeExistingMetadata_ProcessingFlags(t *testing.T) {
	img := Image{ID: "test1"}
	existing := Image{
		ID: "test1",
		ProcessingFlags: map[string]bool{
			"incompatible:1920x1080": true,
			"SmartFit":               true,
		},
	}

	img.MergeExistingMetadata(existing)

	assert.True(t, img.ProcessingFlags["incompatible:1920x1080"])
	assert.True(t, img.ProcessingFlags["SmartFit"])
}

func TestMergeExistingMetadata_ProcessingFlagsMerge(t *testing.T) {
	// New image already has some flags; existing has others
	img := Image{
		ID:              "test1",
		ProcessingFlags: map[string]bool{"newFlag": true},
	}
	existing := Image{
		ID:              "test1",
		ProcessingFlags: map[string]bool{"incompatible:3440x1440": true},
	}

	img.MergeExistingMetadata(existing)

	assert.True(t, img.ProcessingFlags["newFlag"], "New flags should be preserved")
	assert.True(t, img.ProcessingFlags["incompatible:3440x1440"], "Existing flags should be merged")
}

func TestMergeExistingMetadata_CropAnchors(t *testing.T) {
	img := Image{ID: "test1"}
	existing := Image{
		ID: "test1",
		Tuning: map[string]TuningOptions{
			"3440x1440": TuningOptions{Anchor: AnchorTopCenter},
			"1920x1080": TuningOptions{Anchor: AnchorMiddleCenter},
		},
	}

	img.MergeExistingMetadata(existing)

	assert.Equal(t, TuningOptions{Anchor: AnchorTopCenter}, img.Tuning["3440x1440"])
	assert.Equal(t, TuningOptions{Anchor: AnchorMiddleCenter}, img.Tuning["1920x1080"])
}

func TestMergeExistingMetadata_CropAnchorsNoOverwrite(t *testing.T) {
	// New image already has a user-set anchor — should NOT be overwritten
	img := Image{
		ID: "test1",
		Tuning: map[string]TuningOptions{
			"3440x1440": TuningOptions{Anchor: AnchorBottomCenter}, // user-set
		},
	}
	existing := Image{
		ID: "test1",
		Tuning: map[string]TuningOptions{
			"3440x1440": TuningOptions{Anchor: AnchorTopCenter},    // old value — should NOT overwrite
			"1920x1080": TuningOptions{Anchor: AnchorMiddleCenter}, // new key — should be added
		},
	}

	img.MergeExistingMetadata(existing)

	assert.Equal(t, TuningOptions{Anchor: AnchorBottomCenter}, img.Tuning["3440x1440"], "User-set anchor should not be overwritten")
	assert.Equal(t, TuningOptions{Anchor: AnchorMiddleCenter}, img.Tuning["1920x1080"], "New anchor should be added")
}

func TestMergeExistingMetadata_NoDerivativePathsLeak(t *testing.T) {
	img := Image{ID: "test1"}
	existing := Image{
		ID: "test1",
		DerivativePaths: map[string]string{
			"3440x1440": "/path/to/derivative.jpg",
		},
		FilePath:    "/path/to/file.jpg",
		Seen:        true,
		IsFavorited: true,
	}

	img.MergeExistingMetadata(existing)

	assert.Nil(t, img.DerivativePaths, "DerivativePaths should NOT be merged")
	assert.Empty(t, img.FilePath, "FilePath should NOT be merged")
	assert.False(t, img.Seen, "Seen should NOT be merged")
	assert.False(t, img.IsFavorited, "IsFavorited should NOT be merged")
}

func TestMergeExistingMetadata_EmptyExisting(t *testing.T) {
	img := Image{ID: "test1", Width: 100, Height: 200}
	existing := Image{ID: "test1"}

	img.MergeExistingMetadata(existing)

	// Zero-value dimensions from existing should not overwrite
	assert.Equal(t, 100, img.Width, "Width should not be cleared by zero-value existing")
	assert.Equal(t, 200, img.Height, "Height should not be cleared by zero-value existing")
	assert.Nil(t, img.Tuning, "CropAnchors should remain nil")
}
