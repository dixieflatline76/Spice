package googlephotos

import (
	"context"
	"net/http"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
)

func TestProvider_Basics(t *testing.T) {
	cfg := &wallpaper.Config{}
	client := &http.Client{}
	p := NewProvider(cfg, client)

	assert.Equal(t, "GooglePhotos", p.Name())
	assert.Equal(t, "Google Photos", p.Title())
	assert.Equal(t, "https://photos.google.com", p.HomeURL())
}

func TestProvider_ParseURL(t *testing.T) {
	cfg := &wallpaper.Config{}
	client := &http.Client{}
	p := NewProvider(cfg, client)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid URL",
			input:   "googlephotos://some-guid-123",
			want:    "googlephotos://some-guid-123",
			wantErr: false,
		},
		{
			name:    "Invalid Scheme",
			input:   "http://google.com",
			wantErr: true,
		},
		{
			name:    "Invalid URL format",
			input:   "://broken",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseURL() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_EnrichImage(t *testing.T) {
	cfg := &wallpaper.Config{}
	client := &http.Client{}
	p := NewProvider(cfg, client)

	// EnrichImage currently does nothing, but good to ensure it doesn't error
	img, err := p.EnrichImage(context.Background(), provider.Image{}) // Mock helper? No, just provider.Image
	assert.NoError(t, err)
	assert.Empty(t, img.ID)
}
