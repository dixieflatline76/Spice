//go:build darwin

package sysinfo

import "testing"

func TestParseScreenResolution(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantW   int
		wantH   int
		wantErr bool
	}{
		{
			name: "Standard 1080p",
			input: `
Calculated Integrity: 
      Resolution: 1920 x 1080
      UI Looks like: 1920 x 1080
`,
			wantW:   1920,
			wantH:   1080,
			wantErr: false,
		},
		{
			name: "Retina Display",
			input: `
      Resolution: 2880 x 1800 Retina
      UI Looks like: 1440 x 900 @ 2x
`,
			wantW:   2880,
			wantH:   1800,
			wantErr: false,
		},
		{
			name: "Multiple Displays (Matches First)",
			input: `
      Resolution: 2560 x 1440
      UI Looks like: 2560 x 1440
      Resolution: 1920 x 1080
`,
			wantW:   2560,
			wantH:   1440,
			wantErr: false,
		},
		{
			name:    "Invalid Output",
			input:   "No resolution here",
			wantW:   0,
			wantH:   0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH, err := parseScreenResolution(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseScreenResolution() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotW != tt.wantW {
				t.Errorf("parseScreenResolution() gotW = %v, want %v", gotW, tt.wantW)
			}
			if gotH != tt.wantH {
				t.Errorf("parseScreenResolution() gotH = %v, want %v", gotH, tt.wantH)
			}
		})
	}
}
