// Package msa signs in to a Microsoft account as an OAuth 2.0 public client
// (consumers tenant, no client secret): the browser flow with PKCE and a
// loopback redirect (RFC 8252), the device code flow (RFC 8628) and refresh.
package msa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"quartz/internal/core/netx"
)

// Scope asks for Xbox Live sign-in plus a refresh token.
const Scope = "XboxLive.signin XboxLive.offline_access"

// Endpoints are the Microsoft identity URLs. Tests point them at a fake server.
type Endpoints struct {
	Authorize  string
	Token      string
	DeviceCode string
}

// Live is the Microsoft identity platform, consumers tenant.
var Live = Endpoints{ //nolint:gosec // G101: public endpoint URLs, not credentials
	Authorize:  "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize",
	Token:      "https://login.microsoftonline.com/consumers/oauth2/v2.0/token",
	DeviceCode: "https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode",
}

// Errors callers branch on. Anything else is either an *OAuthError or a
// network error (see netx.Classify).
var (
	ErrNotConfigured = errors.New("msa: Microsoft sign-in isn't set up in this build")
	ErrReauth        = errors.New("msa: the sign-in expired; sign in again")
	ErrExpired       = errors.New("msa: the code expired")
	ErrDenied        = errors.New("msa: sign-in was declined")
	ErrTimeout       = errors.New("msa: sign-in timed out")
)

// OAuthError is an error response from the token endpoint.
type OAuthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *OAuthError) Error() string { return "msa: " + e.Code }

// Token is a Microsoft access token and the refresh token that renews it.
type Token struct {
	Access    string
	Refresh   string
	ExpiresAt time.Time
}

// Client runs the flows for one client ID.
type Client struct {
	HTTP      *netx.Client
	ClientID  string
	Endpoints Endpoints

	now  func() time.Time
	wait func(ctx context.Context, d time.Duration) error
}

// New returns a client for the live endpoints. clientID "" makes every flow
// fail with ErrNotConfigured.
func New(h *netx.Client, clientID string) *Client {
	return &Client{HTTP: h, ClientID: clientID, Endpoints: Live, now: time.Now, wait: sleep}
}

// Refresh exchanges a refresh token for a new token pair. invalid_grant (the
// refresh token was revoked, expired or the password changed) and the
// interaction errors return ErrReauth; network trouble is returned as is, so
// the caller can keep the stored token and retry later.
func (c *Client) Refresh(ctx context.Context, refresh string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
}

func (c *Client) token(ctx context.Context, form url.Values) (Token, error) {
	if c.ClientID == "" {
		return Token{}, ErrNotConfigured
	}
	form.Set("client_id", c.ClientID)
	form.Set("scope", Scope)
	var out struct {
		Access    string `json:"access_token"`
		Refresh   string `json:"refresh_token"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if err := c.HTTP.PostForm(ctx, c.Endpoints.Token, form, &out); err != nil {
		return Token{}, oauthError(err)
	}
	if out.Access == "" || out.Refresh == "" {
		return Token{}, errors.New("msa: token response without tokens")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	return Token{Access: out.Access, Refresh: out.Refresh, ExpiresAt: c.now().Add(time.Duration(out.ExpiresIn) * time.Second)}, nil
}

// oauthError turns a 4xx with an OAuth error body into a typed error.
func oauthError(err error) error {
	var se *netx.StatusError
	if !errors.As(err, &se) || se.Code >= 500 {
		return err
	}
	var oe OAuthError
	if json.Unmarshal(se.Body, &oe) != nil || oe.Code == "" {
		return err
	}
	switch oe.Code {
	case "invalid_grant", "interaction_required", "consent_required", "login_required":
		return fmt.Errorf("%w (%s)", ErrReauth, oe.Code)
	case "authorization_declined", "access_denied":
		return ErrDenied
	case "expired_token":
		return ErrExpired
	}
	return &oe
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
