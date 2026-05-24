package dataceen

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
)

// TokenProvider abstracts Azure AD token acquisition. The default
// implementation uses MSAL with the client-credentials flow; tests inject
// fakes via WithTokenProvider.
//
// AcquireToken receives the OAuth scopes plus a forceRefresh flag that
// bypasses MSAL's internal cache (mirrors C#'s AcquireTokenForClient
// .WithForceRefresh(true)). MSAL for Go does not expose a force-refresh
// option as part of its public API, so the default implementation rebuilds
// the underlying confidential.Client on forceRefresh=true to drop its
// in-memory cache and force a fresh STS roundtrip.
type TokenProvider interface {
	AcquireToken(ctx context.Context, scopes []string, forceRefresh bool) (string, error)
}

type msalTokenProvider struct {
	authority    string
	clientID     string
	clientSecret string

	mu  sync.Mutex
	app confidential.Client
}

// NewMSALTokenProvider builds a TokenProvider backed by the Microsoft
// Authentication Library for Go (client-credentials flow). It is the default
// when constructing a Client without WithTokenProvider.
func NewMSALTokenProvider(cfg Config) (TokenProvider, error) {
	if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("dataceen: TenantID, ClientID, ClientSecret are required for MSAL token provider")
	}
	authority := "https://login.microsoftonline.com/" + cfg.TenantID
	tp := &msalTokenProvider{
		authority:    authority,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
	}
	if err := tp.rebuild(); err != nil {
		return nil, err
	}
	return tp, nil
}

func (m *msalTokenProvider) rebuild() error {
	cred, err := confidential.NewCredFromSecret(m.clientSecret)
	if err != nil {
		return fmt.Errorf("dataceen: build client-secret cred: %w", err)
	}
	app, err := confidential.New(m.authority, m.clientID, cred)
	if err != nil {
		return fmt.Errorf("dataceen: confidential.New: %w", err)
	}
	m.app = app
	return nil
}

func (m *msalTokenProvider) AcquireToken(ctx context.Context, scopes []string, forceRefresh bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if forceRefresh {
		if err := m.rebuild(); err != nil {
			return "", err
		}
	}

	res, err := m.app.AcquireTokenByCredential(ctx, scopes)
	if err != nil {
		return "", fmt.Errorf("dataceen: MSAL AcquireTokenByCredential: %w", err)
	}
	if res.AccessToken == "" {
		return "", errors.New("dataceen: MSAL returned empty access token")
	}
	return res.AccessToken, nil
}
