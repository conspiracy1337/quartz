// Package xbox trades a Microsoft access token for an Xbox Live user token
// and then an XSTS token for the Minecraft services relying party.
package xbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"quartz/internal/core/netx"
)

// Endpoints are the Xbox Live auth URLs. Tests point them at a fake server.
type Endpoints struct {
	User string
	XSTS string
}

// Live is Xbox Live.
var Live = Endpoints{
	User: "https://user.auth.xboxlive.com/user/authenticate",
	XSTS: "https://xsts.auth.xboxlive.com/xsts/authorize",
}

// RelyingParty is the audience of the XSTS token Minecraft accepts.
const RelyingParty = "rp://api.minecraftservices.com/"

// Token is an XSTS token with the user hash Minecraft's login needs.
type Token struct {
	Token    string
	UHS      string
	NotAfter time.Time
}

// ErrUHSMismatch means the XSTS token belongs to another user than the
// user token we sent (Prism checks this too).
var ErrUHSMismatch = errors.New("xbox: the XSTS token is for another user")

// Error is an XSTS refusal (HTTP 401 with an XErr code), with a message the
// user can act on and, when there is one, a page that fixes it.
type Error struct {
	XErr    int64
	Reason  string
	Message string
	FixURL  string
}

func (e *Error) Error() string { return "xbox: " + e.Reason + " (" + strconv.FormatInt(e.XErr, 10) + ")" }

// problems maps XErr codes to what the user sees. The fix links are pages that
// open in a browser; Microsoft's own Redirect values are console deep links.
var problems = map[int64]Error{
	2148916227: {Reason: "banned", Message: "This account is banned from Xbox Live.", FixURL: "https://enforcement.xbox.com"},
	2148916229: {Reason: "parental-online", Message: "A parent must allow online play for this account.", FixURL: "https://account.microsoft.com/family/"},
	2148916233: {Reason: "no-xbox-profile", Message: "This Microsoft account has no Xbox profile yet.", FixURL: "https://www.minecraft.net/msaprofile/mygames/editprofile"},
	2148916234: {Reason: "xbox-terms", Message: "You need to accept the Xbox terms first.", FixURL: "https://account.xbox.com/"},
	2148916235: {Reason: "region", Message: "Xbox Live isn't available in your country or region."},
	2148916236: {Reason: "age-verification", Message: "Microsoft needs you to verify your age.", FixURL: "https://login.live.com/login.srf"},
	2148916237: {Reason: "age-verification", Message: "Microsoft needs you to verify your age.", FixURL: "https://login.live.com/login.srf"},
	2148916238: {Reason: "child-account", Message: "Child accounts must be added to a Microsoft family first.", FixURL: "https://account.microsoft.com/family/"},
}

// Client talks to Xbox Live.
type Client struct {
	HTTP      *netx.Client
	Endpoints Endpoints
}

// New returns a client for the live endpoints.
func New(h *netx.Client) *Client { return &Client{HTTP: h, Endpoints: Live} }

type tokenResponse struct {
	NotAfter      time.Time `json:"NotAfter"`
	Token         string    `json:"Token"`
	DisplayClaims struct {
		XUI []struct {
			UHS string `json:"uhs"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

func (r tokenResponse) uhs() string {
	if len(r.DisplayClaims.XUI) == 0 {
		return ""
	}
	return r.DisplayClaims.XUI[0].UHS
}

// Authenticate turns a Microsoft access token into an XSTS token.
func (c *Client) Authenticate(ctx context.Context, msaAccess string) (Token, error) {
	h := http.Header{"X-Xbl-Contract-Version": {"1"}}
	var user tokenResponse
	err := c.HTTP.JSON(ctx, http.MethodPost, c.Endpoints.User, h, map[string]any{
		"Properties": map[string]any{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + msaAccess,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}, &user)
	if err != nil {
		return Token{}, fmt.Errorf("xbox user token: %w", err)
	}
	if user.Token == "" || user.uhs() == "" {
		return Token{}, errors.New("xbox: user token response without a token")
	}

	var xsts tokenResponse
	err = c.HTTP.JSON(ctx, http.MethodPost, c.Endpoints.XSTS, nil, map[string]any{
		"Properties":   map[string]any{"SandboxId": "RETAIL", "UserTokens": []string{user.Token}},
		"RelyingParty": RelyingParty,
		"TokenType":    "JWT",
	}, &xsts)
	if err != nil {
		return Token{}, xstsError(err)
	}
	if xsts.Token == "" {
		return Token{}, errors.New("xbox: XSTS response without a token")
	}
	if xsts.uhs() != user.uhs() {
		return Token{}, ErrUHSMismatch
	}
	return Token{Token: xsts.Token, UHS: xsts.uhs(), NotAfter: xsts.NotAfter}, nil
}

// xstsError reads the XErr code from a 401 body.
func xstsError(err error) error {
	var se *netx.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusUnauthorized {
		return fmt.Errorf("xbox XSTS token: %w", err)
	}
	var body struct {
		XErr int64 `json:"XErr"`
	}
	if json.Unmarshal(se.Body, &body) != nil || body.XErr == 0 {
		return fmt.Errorf("xbox XSTS token: %w", err)
	}
	if p, ok := problems[body.XErr]; ok {
		p.XErr = body.XErr
		return &p
	}
	return &Error{XErr: body.XErr, Reason: "xbox", Message: fmt.Sprintf("Xbox sign-in failed (code %d).", body.XErr)}
}
