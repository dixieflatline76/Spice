package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsStoreDistribution_StandardBuild verifies that IsStoreDistribution()
// returns false when built without appstore or msstore tags.
// The "true" path is verified by building with:
//
//	go build -tags "release msstore" ./cmd/spice
//	go build -tags "release appstore" ./cmd/spice
func TestIsStoreDistribution_StandardBuild(t *testing.T) {
	assert.False(t, IsStoreDistribution(), "Standard build should NOT be a store distribution")
}
