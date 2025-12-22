package googlephotos

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateRandomString(t *testing.T) {
	// Test basic length and character set (Base64URL)
	s, err := generateRandomString(32)
	assert.NoError(t, err)
	// Base64 of 32 bytes is approx 43 chars (32 * 4/3)
	assert.Greater(t, len(s), 32)

	// Check for URL safe characters (Alphanumeric + "-" + "_")
	// Standard Base64 uses "+" and "/", URL safe uses "-" and "_"
	matched, _ := regexp.MatchString(`^[A-Za-z0-9\-\_]+$`, s)
	assert.True(t, matched, "String should be Base64URL safe (no + or /)")
}

func TestGenerateCodeChallenge(t *testing.T) {
	// Known vectors from RFC 7636 Appendix B
	// verifier: dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk
	// challenge: E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expectedChallenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	challenge := generateCodeChallenge(verifier)
	assert.Equal(t, expectedChallenge, challenge)
}

func TestProviderBasics(t *testing.T) {
	p := &Provider{}
	assert.Equal(t, "GooglePhotos", p.Name())
	assert.Equal(t, "https://photos.google.com", p.HomeURL())
}
