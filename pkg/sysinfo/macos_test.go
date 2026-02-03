//go:build darwin
// +build darwin

package sysinfo

import (
	"testing"
)

func TestParseJSONResolution(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		reqWidth  int
		reqHeight int
		wantErr   bool
	}{
		{
			name: "Single Monitor M3",
			json: `{
  "SPDisplaysDataType" : [
    {
      "spdisplays_ndrvs" : [
        {
          "_spdisplays_pixels" : "3420 x 2214",
          "spdisplays_main" : "spdisplays_yes"
        }
      ]
    }
  ]
}`,
			reqWidth:  3420,
			reqHeight: 2214,
			wantErr:   false,
		},
		{
			name: "Multi Monitor M3",
			json: `{
  "SPDisplaysDataType" : [
    {
      "spdisplays_ndrvs" : [
        {
          "_name" : "Color LCD",
          "_spdisplays_pixels" : "3420 x 2214",
          "spdisplays_main" : "spdisplays_yes"
        },
        {
          "_name" : "U34G2G1",
          "_spdisplays_pixels" : "3440 x 1440"
        }
      ]
    }
  ]
}`,
			reqWidth:  3420,
			reqHeight: 2214,
			wantErr:   false,
		},
		{
			name: "No Main Display Fallback",
			json: `{
  "SPDisplaysDataType" : [
    {
      "spdisplays_ndrvs" : [
        {
          "_spdisplays_pixels" : "1920 x 1080"
        }
      ]
    }
  ]
}`,
			reqWidth:  1920,
			reqHeight: 1080,
			wantErr:   false,
		},
		{
			name:    "Invalid JSON",
			json:    `{"invalid": "format"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, err := parseJSONResolution([]byte(tt.json))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONResolution() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if w != tt.reqWidth || h != tt.reqHeight {
					t.Errorf("parseJSONResolution() got = %dx%d, want %dx%d", w, h, tt.reqWidth, tt.reqHeight)
				}
			}
		})
	}
}
