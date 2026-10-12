package msa

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

// microsoftLink is the short verification page. It accepts ?otc=<code>, which
// pre-fills the code, so the QR code saves typing (as Prism does).
const microsoftLink = "https://www.microsoft.com/link"

// DeviceCode is a pending device sign-in to show to the user.
type DeviceCode struct {
	UserCode        string
	VerificationURI string
	// QRURL is what the QR code encodes: the verification page, with the code
	// pre-filled when Microsoft supports it.
	QRURL     string
	ExpiresAt time.Time

	deviceCode string
	interval   time.Duration
}

// StartDevice asks Microsoft for a device code.
func (c *Client) StartDevice(ctx context.Context) (DeviceCode, error) {
	if c.ClientID == "" {
		return DeviceCode{}, ErrNotConfigured
	}
	var out struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int64  `json:"expires_in"`
		Interval        int64  `json:"interval"`
	}
	form := url.Values{"client_id": {c.ClientID}, "scope": {Scope}}
	if err := c.HTTP.PostForm(ctx, c.Endpoints.DeviceCode, form, &out); err != nil {
		return DeviceCode{}, oauthError(err)
	}
	// The page is shown and encoded in a QR code, so it must be https.
	if out.DeviceCode == "" || out.UserCode == "" || !strings.HasPrefix(out.VerificationURI, "https://") {
		return DeviceCode{}, errors.New("msa: malformed device code response")
	}
	dc := DeviceCode{
		UserCode:        out.UserCode,
		VerificationURI: out.VerificationURI,
		QRURL:           out.VerificationURI,
		ExpiresAt:       c.now().Add(time.Duration(max(out.ExpiresIn, 60)) * time.Second),
		deviceCode:      out.DeviceCode,
		interval:        time.Duration(max(out.Interval, 1)) * time.Second,
	}
	if out.VerificationURI == microsoftLink {
		dc.QRURL = microsoftLink + "?otc=" + url.QueryEscape(out.UserCode)
	}
	return dc, nil
}

// PollDevice polls until the user finishes, declines, or the code expires.
// authorization_pending keeps polling; slow_down adds 5 s to the interval.
func (c *Client) PollDevice(ctx context.Context, dc DeviceCode) (Token, error) {
	interval := dc.interval
	for {
		if !c.now().Before(dc.ExpiresAt) {
			return Token{}, ErrExpired
		}
		if err := c.wait(ctx, interval); err != nil {
			return Token{}, err
		}
		tok, err := c.token(ctx, url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {dc.deviceCode},
		})
		var oe *OAuthError
		switch {
		case err == nil:
			return tok, nil
		case errors.As(err, &oe) && oe.Code == "authorization_pending":
		case errors.As(err, &oe) && oe.Code == "slow_down":
			interval += 5 * time.Second
		default:
			return Token{}, err
		}
	}
}
