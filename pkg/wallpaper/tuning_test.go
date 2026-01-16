package wallpaper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultTuningConfig(t *testing.T) {
	cfg := DefaultTuningConfig()

	// Verify defaults for refactored params
	assert.Equal(t, 10.0, cfg.FaceDetectConfidence, "FaceDetectConfidence should be 10.0")
	assert.Equal(t, 1, cfg.FaceDetectMinSizePct, "FaceDetectMinSizePct should be 1")
	assert.Equal(t, 0.1, cfg.FaceDetectShift, "FaceDetectShift should be 0.1")

	// Verify other critical defaults
	assert.Equal(t, 0.05, cfg.MinEnergyThreshold, "MinEnergyThreshold should be 0.05")
	assert.Equal(t, 0.2, cfg.FeetGuardHighEnergyThreshold, "FeetGuardHighEnergyThreshold should be 0.2")
}
