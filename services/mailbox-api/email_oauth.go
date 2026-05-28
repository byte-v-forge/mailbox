package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/byte-v-forge/common-lib/envx"
	"golang.org/x/oauth2"
)

type OAuthManager struct {
	mu           sync.Mutex
	refreshToken string
	accessToken  string
	expiresAt    time.Time
	clientID     string
	scope        string
	tokenURL     string
	httpClient   *http.Client
}

func NewOAuthManager(refreshToken string) *OAuthManager {
	timeout := envx.Int("OUTLOOK_HTTP_TIMEOUT_SECONDS", defaultHTTPTimeoutSeconds)
	if timeout <= 0 {
		timeout = defaultHTTPTimeoutSeconds
	}
	scope := normalizeScope(envx.StringDefault("OUTLOOK_OAUTH_SCOPE", defaultOAuthScope))
	if scope == "" {
		scope = defaultOAuthScope
	}
	return &OAuthManager{
		refreshToken: strings.TrimSpace(refreshToken),
		clientID:     envx.StringDefault("OUTLOOK_OAUTH_CLIENT_ID", defaultOAuthClientID),
		scope:        scope,
		tokenURL:     envx.StringDefault("OUTLOOK_OAUTH_TOKEN_URL", defaultTokenURL),
		httpClient:   &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

func (m *OAuthManager) GetAccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.accessToken != "" && time.Now().Before(m.expiresAt.Add(-60*time.Second)) {
		return m.accessToken, nil
	}
	return m.refreshLocked(ctx)
}

func (m *OAuthManager) RefreshAccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refreshLocked(ctx)
}

func (m *OAuthManager) CurrentTokens() (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refreshToken, m.accessToken
}

func (m *OAuthManager) refreshLocked(ctx context.Context) (string, error) {
	if strings.TrimSpace(m.refreshToken) == "" {
		return "", fmt.Errorf("refresh token is missing")
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.httpClient)
	cfg := m.oauthConfig()
	token, err := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: m.refreshToken}).Token()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return "", fmt.Errorf("token refresh returned empty access token")
	}
	m.accessToken = token.AccessToken
	if strings.TrimSpace(token.RefreshToken) != "" {
		m.refreshToken = token.RefreshToken
	}
	m.expiresAt = token.Expiry
	if m.expiresAt.IsZero() {
		m.expiresAt = time.Now().Add(time.Hour)
	}
	return m.accessToken, nil
}

func (m *OAuthManager) oauthConfig() oauth2.Config {
	return oauth2.Config{
		ClientID: m.clientID,
		Scopes:   strings.Fields(m.scope),
		Endpoint: oauth2.Endpoint{
			TokenURL:  m.tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}
