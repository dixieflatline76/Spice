package unsplash

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
)

// UnsplashAuthenticator handles the OAuth flow for Unsplash.
type UnsplashAuthenticator struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
}

// NewUnsplashAuthenticator creates a new UnsplashAuthenticator.
func NewUnsplashAuthenticator(cfg *wallpaper.Config, client *http.Client) *UnsplashAuthenticator {
	return &UnsplashAuthenticator{
		cfg:        cfg,
		httpClient: client,
	}
}

// StartOAuthFlow initiates the OAuth flow.
// It starts a local server, opens the browser, and waits for the callback.
func (a *UnsplashAuthenticator) StartOAuthFlow(openURLFunc func(*url.URL) error) error {
	state, err := generateRandomString(32)
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	// Create a channel to receive the auth code
	codeChan := make(chan string)
	errChan := make(chan error)

	// Start local server
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Verify state
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

		// Send success response to browser
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`
			<html>
			<body style="font-family: sans-serif; text-align: center; padding-top: 50px;">
				<h1 style="color: #4CAF50;">Connected!</h1>
				<p>Spice has successfully connected to Unsplash.</p>
				<p>You can close this window now.</p>
				<script>window.close()</script>
			</body>
			</html>
		`)); err != nil {
			log.Printf("Failed to write response: %v", err)
		}
		codeChan <- code
	})

	server := &http.Server{
		Addr:              ":10999",
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second, // G112: Potential Slowloris Attack
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("OAuth server error: %v", err)
			errChan <- err
		}
	}()

	// Construct Auth URL
	authURL, _ := url.Parse(UnsplashAuthURL)
	q := authURL.Query()
	q.Set("client_id", UnsplashClientID)
	q.Set("redirect_uri", UnsplashRedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "public")
	q.Set("state", state)
	authURL.RawQuery = q.Encode()

	// Open browser
	log.Printf("Opening browser for OAuth: %s", authURL.String())
	if err := openURLFunc(authURL); err != nil {
		server.Close()
		return fmt.Errorf("failed to open browser: %w", err)
	}

	// Wait for code or timeout
	var code string
	select {
	case code = <-codeChan:
		// Success
	case err := <-errChan:
		server.Close()
		return err
	case <-time.After(2 * time.Minute):
		server.Close()
		return fmt.Errorf("authentication timed out")
	}

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Failed to shutdown server: %v", err)
	}

	// Exchange code for token
	return a.exchangeCodeForToken(code)
}

// exchangeCodeForToken exchanges the authorization code for an access token.
func (a *UnsplashAuthenticator) exchangeCodeForToken(code string) error {
	data := url.Values{}
	data.Set("client_id", UnsplashClientID)
	data.Set("client_secret", UnsplashClientSecret)
	data.Set("redirect_uri", UnsplashRedirectURI)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")

	resp, err := a.httpClient.PostForm(UnsplashTokenURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed with status: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	// Save token
	a.cfg.SetUnsplashToken(result.AccessToken)
	log.Print("Unsplash authentication successful. Token saved.")

	return nil
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
