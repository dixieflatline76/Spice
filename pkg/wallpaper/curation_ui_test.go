//go:build !linux

package wallpaper

import (
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/curation"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/stretchr/testify/assert"
)

func TestBuildCuratedUIItem_WithPreview(t *testing.T) {
	// Setup mocks
	p := new(MockImageProvider)
	p.On("ID").Return("testprov")
	cfg := &Config{} // Get default config

	// Test entry with preview items
	entry := curation.CollectionEntry{
		Key:  "test_collection",
		Name: "Test Collection",
		Type: "curated",
		IDs:  []string{"img1", "img2"},
	}

	// Build the item
	item := buildCuratedUIItem(p, nil, cfg, entry)

	// In TDD phase 1, this should fail because it currently returns a schema.BoolItem,
	// but we expect a schema.HorizontalRowItem that encapsulates the BoolItem and a ButtonItem.

	rowItem, ok := item.(schema.HorizontalRowItem)
	assert.True(t, ok, "Expected buildCuratedUIItem to return a schema.HorizontalRowItem for entries with items")

	if ok {
		assert.Len(t, rowItem.Items, 2, "Expected HorizontalRowItem to contain 2 items (toggle and button)")

		_, isBoolItem := rowItem.Items[0].(schema.BoolItem)
		assert.True(t, isBoolItem, "Expected first item to be a BoolItem")

		btnItem, isBtnItem := rowItem.Items[1].(schema.ButtonItem)
		assert.True(t, isBtnItem, "Expected second item to be a ButtonItem")

		if isBtnItem {
			assert.Equal(t, "Preview", btnItem.ButtonText, "Expected button text to be 'Preview'")
		}
	}
}
