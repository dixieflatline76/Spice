package googlephotos

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
)

// Authenticator handles the OAuth flow for Google Photos.
type Authenticator struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
}

// NewAuthenticator creates a new Google Photos Authenticator.
func NewAuthenticator(cfg *wallpaper.Config, client *http.Client) *Authenticator {
	return &Authenticator{
		cfg:        cfg,
		httpClient: client,
	}
}

// StartOAuthFlow initiates the OAuth flow using PKCE.
func (a *Authenticator) StartOAuthFlow(openURLFunc func(*url.URL) error) error {
	// 1. Generate PKCE Verifier and Challenge
	verifier, err := generateRandomString(32)
	if err != nil {
		return fmt.Errorf("failed to generate verifier: %w", err)
	}
	challenge := generateCodeChallenge(verifier)

	state, err := generateRandomString(32)
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	// 2. Setup Local Server for Callback
	codeChan := make(chan string)
	errChan := make(chan error)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid state", http.StatusBadRequest)
			errChan <- fmt.Errorf("invalid state parameter")
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing code", http.StatusBadRequest)
			errChan <- fmt.Errorf("missing code parameter")
			return
		}

		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`
			<html>
			<head>
				<meta charset="UTF-8">
				<title>Spice Connected</title>
				<link href="https://fonts.googleapis.com/css2?family=Press+Start+2P&display=swap" rel="stylesheet">
				<style>
					body {
						background: linear-gradient(135deg, #FFC107 0%, #FF9800 100%);
						font-family: 'Press Start 2P', cursive;
						color: #5D4037;
						text-align: center;
						display: flex;
						flex-direction: column;
						align-items: center;
						justify-content: center;
						height: 100vh;
						margin: 0;
						overflow: hidden;
					}
					h1 {
						font-size: 48px;
						color: #FFF;
						text-shadow: 4px 4px 0px #3E2723;
						margin-bottom: 20px;
						z-index: 10;
					}
					.subtitle {
						font-size: 18px;
						margin-bottom: 40px;
						line-height: 1.5;
					}
					.success-box {
						background: rgba(255, 255, 255, 0.9);
						padding: 30px;
						border-radius: 4px;
						box-shadow: 8px 8px 0px #3E2723;
						border: 4px solid #3E2723;
						z-index: 10;
						position: relative;
					}
					.btn {
						background: #D32F2F;
						color: white;
						border: none;
						padding: 15px 30px;
						font-family: 'Press Start 2P', cursive;
						font-size: 14px;
						cursor: pointer;
						margin-top: 20px;
						box-shadow: 4px 4px 0px #3E2723;
						text-decoration: none;
						display: inline-block;
					}
					.btn:active {
						transform: translate(2px, 2px);
						box-shadow: 2px 2px 0px #3E2723;
					}
					/* Floating Chilies */
					.chili {
						position: absolute;
						font-family: "Segoe UI Emoji", "Apple Color Emoji", "Noto Color Emoji", sans-serif;
						font-size: 40px;
						opacity: 0.8;
						animation: float 6s ease-in-out infinite;
						z-index: 1;
					}
					@keyframes float {
						0% { transform: translateY(0px) rotate(0deg); }
						50% { transform: translateY(-20px) rotate(10deg); }
						100% { transform: translateY(0px) rotate(0deg); }
					}
				</style>
			</head>
			<body>
				<!-- Floating Chilies -->
				<div class="chili" style="top: 10%; left: 10%; animation-delay: 0s;">🌶️</div>
				<div class="chili" style="top: 20%; right: 15%; animation-delay: 1s;">🌶️</div>
				<div class="chili" style="bottom: 15%; left: 20%; animation-delay: 2s; font-size: 30px;">🌶️</div>
				<div class="chili" style="bottom: 25%; right: 10%; animation-delay: 3s; font-size: 50px;">🌶️</div>
				<div class="chili" style="top: 50%; left: 5%; animation-delay: 4s; font-size: 25px;">🌶️</div>

				<h1>Spice</h1>
				<div class="success-box">
					<div style="font-size: 24px; color: #388E3C; margin-bottom: 20px;">Connected!</div>
					<div class="subtitle">Successfully linked to<br>Google Photos</div>
					<div style="font-size: 14px; opacity: 0.9; margin-top: 20px; font-weight: bold; color: #5D4037;">
						Please close this tab<br>and return to Spice.
					</div>
				</div>
				<p style="margin-top: 50px; font-size: 12px; color: #FFF; text-shadow: 2px 2px 0px #3E2723; z-index: 10;">Add a little Spice to your screen.</p>
			</body>
			</html>
		`)); err != nil {
			log.Printf("Failed to write callback response: %v", err)
		}
		codeChan <- code
	})

	// Use port 10999 to match GooglePhotosRedirectURI
	server := &http.Server{
		Addr:              ":10999",
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second, // G112: Potential Slowloris Attack
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// 3. Construct Auth URL
	authURL, _ := url.Parse(GooglePhotosAuthURL)
	q := authURL.Query()
	q.Set("client_id", GoogleClientID)
	q.Set("redirect_uri", GooglePhotosRedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "https://www.googleapis.com/auth/photospicker.mediaitems.readonly")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("prompt", "consent")
	q.Set("access_type", "offline")
	authURL.RawQuery = q.Encode()

	// 4. Open Browser
	log.Printf("Opening browser for OAuth: %s", authURL.String())
	if err := openURLFunc(authURL); err != nil {
		server.Close()
		return fmt.Errorf("failed to open browser: %w", err)
	}

	// 5. Wait for Code
	var code string
	select {
	case code = <-codeChan:
	case err := <-errChan:
		server.Close()
		return err
	case <-time.After(2 * time.Minute):
		server.Close()
		return fmt.Errorf("authentication timed out")
	}

	// Shutdown server gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Failed to shutdown callback server gracefully: %v", err)
	}

	// 6. Exchange Code for Token
	return a.exchangeCodeForToken(code, verifier)
}

func (a *Authenticator) exchangeCodeForToken(code, verifier string) error {
	data := url.Values{}
	data.Set("client_id", GoogleClientID)
	data.Set("client_secret", GoogleClientSecret) // Required for Desktop apps even with PKCE
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", GooglePhotosRedirectURI)
	data.Set("code_verifier", verifier)

	resp, err := a.httpClient.PostForm(GooglePhotosTokenURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed with status: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	// Save tokens
	a.cfg.SetGooglePhotosToken(result.AccessToken)
	if result.RefreshToken != "" {
		a.cfg.SetGooglePhotosRefreshToken(result.RefreshToken)
	}
	// Calculate expiry
	expiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	a.cfg.SetGooglePhotosTokenExpiry(expiry)

	log.Print("Google Photos authentication successful.")
	return nil
}

// RefreshToken refreshes the access token if needed.
func (a *Authenticator) RefreshToken() error {
	refreshToken := a.cfg.GetGooglePhotosRefreshToken()
	if refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	data := url.Values{}
	data.Set("client_id", GoogleClientID)
	data.Set("client_secret", GoogleClientSecret)
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := a.httpClient.PostForm(GooglePhotosTokenURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed with status: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	a.cfg.SetGooglePhotosToken(result.AccessToken)
	expiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	a.cfg.SetGooglePhotosTokenExpiry(expiry)

	return nil
}

// EnsureValidToken checks if the current token is valid, and refreshes it if not.
func (a *Authenticator) EnsureValidToken() error {
	expiry := a.cfg.GetGooglePhotosTokenExpiry()
	// Refresh if expired or expiring within 5 minutes
	if time.Now().After(expiry.Add(-5 * time.Minute)) {
		return a.RefreshToken()
	}
	return nil
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	// PKCE requires Base64URL encoding without padding
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// RevokeToken invalidates the given access token or refresh token.
func (a *Authenticator) RevokeToken(token string) error {
	if token == "" {
		return nil
	}
	// Google revocation endpoint
	revokeURL := "https://oauth2.googleapis.com/revoke"

	// Prepare POST data
	data := url.Values{}
	data.Set("token", token)

	resp, err := a.httpClient.PostForm(revokeURL, data)
	if err != nil {
		return fmt.Errorf("failed to send revocation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Just log, don't fail hard if it's already invalid
		log.Printf("Token revocation returned status %d. It may already be invalid.", resp.StatusCode)
	}
	return nil
}
