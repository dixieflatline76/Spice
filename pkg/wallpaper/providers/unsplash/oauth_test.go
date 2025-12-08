package unsplash

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateRandomString(t *testing.T) {
	// Test length
	s, err := generateRandomString(32)
	assert.NoError(t, err)
	// Base64 encoding increases length. 32 bytes -> 43 chars (RawURL)
	assert.Greater(t, len(s), 32)

	// Test randomness (simple check)
	s2, err := generateRandomString(32)
	assert.NoError(t, err)
	assert.NotEqual(t, s, s2)
}
