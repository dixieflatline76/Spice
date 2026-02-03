//go:build darwin
// +build darwin

package wallpaper

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSystemProfiler_MultiMonitor(t *testing.T) {
	jsonInput := `{
  "SPDisplaysDataType" : [
    {
      "spdisplays_ndrvs" : [
        {
          "_name" : "Color LCD",
          "_spdisplays_pixels" : "3420 x 2214"
        },
        {
          "_name" : "U34G2G1",
          "_spdisplays_pixels" : "3440 x 1440"
        }
      ]
    }
  ]
}`

	m := &macOSOS{}
	monitors, err := m.parseSystemProfiler(jsonInput)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(monitors))

	assert.Equal(t, 0, monitors[0].ID)
	assert.Equal(t, "Color LCD", monitors[0].Name)
	assert.Equal(t, image.Rect(0, 0, 3420, 2214), monitors[0].Rect)

	assert.Equal(t, 1, monitors[1].ID)
	assert.Equal(t, "U34G2G1", monitors[1].Name)
	assert.Equal(t, image.Rect(0, 0, 3440, 1440), monitors[1].Rect)
}

func TestParseSystemProfiler_MultiGPU(t *testing.T) {
	jsonInput := `{
  "SPDisplaysDataType" : [
    {
      "spdisplays_ndrvs" : [
        {
          "_name" : "Built-in Display",
          "_spdisplays_pixels" : "2880 x 1800"
        }
      ]
    },
    {
      "spdisplays_ndrvs" : [
        {
          "_name" : "External Display",
          "_spdisplays_pixels" : "1920 x 1080"
        }
      ]
    }
  ]
}`

	m := &macOSOS{}
	monitors, err := m.parseSystemProfiler(jsonInput)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(monitors))

	assert.Equal(t, 0, monitors[0].ID)
	assert.Equal(t, "Built-in Display", monitors[0].Name)
	assert.Equal(t, 1, monitors[1].ID)
	assert.Equal(t, "External Display", monitors[1].Name)
}
