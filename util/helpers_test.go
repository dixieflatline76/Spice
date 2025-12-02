package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringsToStringers(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "Single item",
			input:    []string{"hello"},
			expected: []string{"hello"},
		},
		{
			name:     "Multiple items",
			input:    []string{"one", "two", "three"},
			expected: []string{"one", "two", "three"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringsToStringers(tt.input)
			assert.Equal(t, len(tt.expected), len(result))
			for i, s := range result {
				assert.Equal(t, tt.expected[i], s.String())
				assert.Implements(t, (*fmt.Stringer)(nil), s)
			}
		})
	}
}
