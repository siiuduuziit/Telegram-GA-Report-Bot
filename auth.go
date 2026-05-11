package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/option"
)

func newGAService(ctx context.Context, cfg appConfig) (*analyticsdata.Service, error) {
	tokenSource, err := newOAuthTokenSource(ctx, cfg.GAOAuthClientSecretFile, cfg.GAOAuthTokenFile)
	if err != nil {
		return nil, fmt.Errorf("oauth init failed: %w", err)
	}
	return analyticsdata.NewService(ctx, option.WithTokenSource(tokenSource))
}

func newOAuthTokenSource(ctx context.Context, clientSecretFile, tokenFile string) (oauth2.TokenSource, error) {
	b, err := os.ReadFile(clientSecretFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read oauth client secret file: %w", err)
	}

	cfg, err := google.ConfigFromJSON(b, analyticsdata.AnalyticsReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("invalid oauth client config: %w", err)
	}

	token, err := readOAuthToken(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read oauth token file: %w", err)
	}

	return cfg.TokenSource(ctx, token), nil
}

func readOAuthToken(path string) (*oauth2.Token, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var token oauth2.Token
	if err := json.Unmarshal(b, &token); err != nil {
		return nil, fmt.Errorf("invalid oauth token json: %w", err)
	}

	if token.AccessToken == "" && token.RefreshToken == "" {
		return nil, fmt.Errorf("oauth token missing both access_token and refresh_token")
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if token.Expiry.IsZero() {
		token.Expiry = time.Now().Add(-time.Minute)
	}

	return &token, nil
}

func bootstrapOAuthToken(ctx context.Context, clientSecretFile, tokenFile string) error {
	b, err := os.ReadFile(clientSecretFile)
	if err != nil {
		return fmt.Errorf("cannot read oauth client secret file: %w", err)
	}

	cfg, err := google.ConfigFromJSON(b, analyticsdata.AnalyticsReadonlyScope)
	if err != nil {
		return fmt.Errorf("invalid oauth client config: %w", err)
	}

	redirectURL := "http://127.0.0.1:8085/callback"
	cfg.RedirectURL = redirectURL
	state := fmt.Sprintf("state-%d", time.Now().UnixNano())
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			errCh <- fmt.Errorf("oauth state mismatch")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- fmt.Errorf("oauth callback missing code")
			return
		}
		_, _ = w.Write([]byte("OAuth success. You can close this tab and return to terminal."))
		codeCh <- code
	})

	server := &http.Server{
		Addr:    "127.0.0.1:8085",
		Handler: mux,
	}

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", server.Addr, err)
	}
	defer ln.Close()

	go func() {
		if serveErr := server.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("Open this URL in your browser and approve access:\n%s\n", authURL)
	log.Println("If browser callback does not work, paste the authorization code manually:")
	fmt.Print("Auth code (optional): ")

	manualCodeCh := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		manualCodeCh <- strings.TrimSpace(line)
	}()

	var code string
	select {
	case code = <-codeCh:
	case code = <-manualCodeCh:
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	if code == "" {
		return fmt.Errorf("empty authorization code")
	}

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	tokenJSON, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode oauth token: %w", err)
	}

	if err := os.WriteFile(tokenFile, tokenJSON, 0600); err != nil {
		return fmt.Errorf("cannot write oauth token file: %w", err)
	}

	log.Printf("OAuth token saved to %s", tokenFile)
	return nil
}
