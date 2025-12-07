package util

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/dixieflatline76/Spice/config"
	"github.com/stretchr/testify/assert"
)

func TestEULA(t *testing.T) {
	// Setup
	prefs := test.NewApp().Preferences()
	// Ensure clean state
	prefs.SetString(EULAPreferenceKey, "")

	// Mock AppVersion for consistent testing
	originalVersion := config.AppVersion
	config.AppVersion = "1.0.0"
	defer func() { config.AppVersion = originalVersion }()

	// Test 1: Initially not accepted
	assert.False(t, HasAcceptedEULA(prefs), "EULA should not be accepted initially")

	// Test 2: Mark as accepted
	MarkEULAAccepted(prefs)
	assert.True(t, HasAcceptedEULA(prefs), "EULA should be accepted after marking")

	// Test 3: Version mismatch invalidates acceptance
	config.AppVersion = "1.0.1"
	assert.False(t, HasAcceptedEULA(prefs), "EULA should be invalid after version change")

	// Test 4: Re-accept new version
	MarkEULAAccepted(prefs)
	assert.True(t, HasAcceptedEULA(prefs), "EULA should be accepted after re-marking for new version")

	// Test 5: Tampering detection (manual modification of prefs)
	// We can't easily modify the internal JSON of the memory prefs without using the key,
	// but we can simulate a bad hash by setting a valid JSON with invalid hash.
	// However, since we rely on the internal implementation of MarkEULAAccepted,
	// the best way to test tampering is to verify the hash logic itself or
	// manually set a bad JSON string.

	badJson := `{"eula_version":"1.0.1","acceptance_timestamp":"2023-01-01T00:00:00Z","hash":"bad_hash"}`
	prefs.SetString(EULAPreferenceKey, badJson)
	assert.False(t, HasAcceptedEULA(prefs), "EULA should be invalid if hash is tampered")
}

func TestGenerateEULAHash(t *testing.T) {
	h1 := generateEULAHash("text", "v1")
	h2 := generateEULAHash("text", "v1")
	h3 := generateEULAHash("text", "v2")

	assert.Equal(t, h1, h2, "Hash should be deterministic")
	assert.NotEqual(t, h1, h3, "Hash should change with version")
}
