package util

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/dixieflatline76/Spice/config"
	"github.com/stretchr/testify/assert"
)

// MockRoundTripper implements http.RoundTripper
type MockRoundTripper struct {
	Response *http.Response
	Err      error
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.Response, m.Err
}

func TestCheckForUpdates(t *testing.T) {
	// Save original version
	originalVersion := config.AppVersion
	defer func() { config.AppVersion = originalVersion }()

	tests := []struct {
		name            string
		currentVersion  string
		responseBody    string
		statusCode      int
		expectUpdate    bool
		expectError     bool
		expectedVersion string
	}{
		{
			name:            "Update Available",
			currentVersion:  "v1.0.0",
			responseBody:    `{"tag_name": "v1.1.0", "html_url": "http://release", "body": "notes"}`,
			statusCode:      200,
			expectUpdate:    true,
			expectError:     false,
			expectedVersion: "v1.1.0",
		},
		{
			name:            "No Update Available",
			currentVersion:  "v1.1.0",
			responseBody:    `{"tag_name": "v1.1.0", "html_url": "http://release", "body": "notes"}`,
			statusCode:      200,
			expectUpdate:    false,
			expectError:     false,
			expectedVersion: "v1.1.0",
		},
		{
			name:            "Newer Local Version",
			currentVersion:  "v2.0.0",
			responseBody:    `{"tag_name": "v1.1.0", "html_url": "http://release", "body": "notes"}`,
			statusCode:      200,
			expectUpdate:    false,
			expectError:     false,
			expectedVersion: "v1.1.0",
		},
		{
			name:            "API Error",
			currentVersion:  "v1.0.0",
			responseBody:    `{"message": "Not Found"}`,
			statusCode:      404,
			expectUpdate:    false,
			expectError:     true, // GitHub client returns error on non-2xx usually, or we might need to handle it
			expectedVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.AppVersion = tt.currentVersion

			// Create mock client
			mockTransport := &MockRoundTripper{
				Response: &http.Response{
					StatusCode: tt.statusCode,
					Body:       io.NopCloser(bytes.NewBufferString(tt.responseBody)),
					Header:     make(http.Header),
				},
			}
			client := &http.Client{Transport: mockTransport}

			result, err := CheckForUpdates(client)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if result != nil {
					assert.Equal(t, tt.expectUpdate, result.UpdateAvailable)
					assert.Equal(t, tt.expectedVersion, result.LatestVersion)
				}
			}
		})
	}
}
