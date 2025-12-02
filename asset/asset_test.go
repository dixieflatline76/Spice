package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssetManager(t *testing.T) {
	am := NewManager()

	t.Run("GetImage", func(t *testing.T) {
		// Test loading an existing image
		img, err := am.GetImage("splash.png")
		assert.NoError(t, err)
		assert.NotNil(t, img)

		// Test loading a non-existent image
		_, err = am.GetImage("non_existent.png")
		assert.Error(t, err)
	})

	t.Run("GetIcon", func(t *testing.T) {
		// Test loading an existing icon
		icon, err := am.GetIcon("tray.png")
		assert.NoError(t, err)
		assert.NotNil(t, icon)
		assert.Equal(t, "tray.png", icon.Name())
		assert.NotEmpty(t, icon.Content())

		// Test loading a non-existent icon
		_, err = am.GetIcon("non_existent.png")
		assert.Error(t, err)
	})

	t.Run("GetText", func(t *testing.T) {
		// Test loading an existing text file
		text, err := am.GetText("eula.txt")
		assert.NoError(t, err)
		assert.NotEmpty(t, text)

		// Test loading a non-existent text file
		_, err = am.GetText("non_existent.txt")
		assert.Error(t, err)
	})
}
