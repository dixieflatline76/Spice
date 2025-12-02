package setting

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockStringer struct {
	val string
}

func (m mockStringer) String() string {
	return m.val
}

func TestStringOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    []fmt.Stringer
		expected []string
	}{
		{
			name:     "Empty slice",
			input:    []fmt.Stringer{},
			expected: []string{},
		},
		{
			name: "Single item",
			input: []fmt.Stringer{
				mockStringer{val: "Option 1"},
			},
			expected: []string{"Option 1"},
		},
		{
			name: "Multiple items",
			input: []fmt.Stringer{
				mockStringer{val: "Option 1"},
				mockStringer{val: "Option 2"},
				mockStringer{val: "Option 3"},
			},
			expected: []string{"Option 1", "Option 2", "Option 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringOptions(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
