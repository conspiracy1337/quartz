// Package mcapi is the client for Minecraft's web services: sign-in with an
// Xbox token, ownership, the player profile, skins and capes, and public
// player lookups. It is shared by sign-in (auth) and the skin library.
package mcapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"
	"time"

	"quartz/internal/core/netx"
)

// Endpoints are the service base URLs. Tests point them at a fake server.
type Endpoints struct {
	Services string // api.minecraftservices.com
	Mojang   string // api.mojang.com
	Session  string // sessionserver.mojang.com
}

// Live is Mojang's production services.
var Live = Endpoints{
	Services: "https://api.minecraftservices.com",
	Mojang:   "https://api.mojang.com",
	Session:  "https://sessionserver.mojang.com",
}

// Errors callers branch on.
var (
	ErrNotOwned  = errors.New("mcapi: this account doesn't own Minecraft: Java Edition")
	ErrNoProfile = errors.New("mcapi: this account has no Minecraft profile yet")
	ErrNoPlayer  = errors.New("mcapi: no player with that name")
	ErrTexture   = errors.New("mcapi: not a Minecraft texture URL")
)

// Token is a Minecraft access token.
type Token struct {
	Access    string
	ExpiresAt time.Time
	XUID      string // Xbox user ID, read from the token's claims
}

// Skin is one of a profile's skins; only the ACTIVE one is worn.
type Skin struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	URL     string `json:"url"`
	Variant string `json:"variant"` // CLASSIC | SLIM
}

// Cape is a cape the account owns.
type Cape struct {
	ID    string `json:"id"`
	State string `json:"state"`
	URL   string `json:"url"`
	Alias string `json:"alias"`
}

// Profile is the signed-in player.
type Profile struct {
	ID    string `json:"id"` // UUID without dashes
	Name  string `json:"name"`
	Skins []Skin `json:"skins"`
	Capes []Cape `json:"capes"`
}

// ActiveSkin returns the worn skin, if the profile has one.
func (p Profile) ActiveSkin() (Skin, bool) {
	for _, s := range p.Skins {
		if s.State == "ACTIVE" {
			return s, true
		}
	}
	return Skin{}, false
}

// Player is another player's public look.
type Player struct {
	ID      string
	Name    string
	SkinURL string // "" for the default skin
	Slim    bool
	CapeURL string
}

// Client talks to the Minecraft services.
type Client struct {
	HTTP      *netx.Client
	Endpoints Endpoints
	now       func() time.Time
}

// New returns a client for the live services.
func New(h *netx.Client) *Client { return &Client{HTTP: h, Endpoints: Live, now: time.Now} }

func bearer(access string) http.Header { return http.Header{"Authorization": {"Bearer " + access}} }

// LoginWithXbox exchanges an XSTS token for a Minecraft access token.
func (c *Client) LoginWithXbox(ctx context.Context, uhs, xsts string) (Token, error) {
	var out struct {
		Access    string `json:"access_token"`
		ExpiresIn int64  `json:"expires_in"`
	}
	in := map[string]string{"identityToken": "XBL3.0 x=" + uhs + ";" + xsts}
	if err := c.HTTP.JSON(ctx, http.MethodPost, c.Endpoints.Services+"/authentication/login_with_xbox", nil, in, &out); err != nil {
		return Token{}, fmt.Errorf("minecraft sign-in: %w", err)
	}
	if out.Access == "" {
		return Token{}, errors.New("mcapi: sign-in response without a token")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 86400
	}
	return Token{Access: out.Access, ExpiresAt: c.now().Add(time.Duration(out.ExpiresIn) * time.Second), XUID: xuid(out.Access)}, nil
}

// xuid reads the xuid claim of a Minecraft token. The token came straight
// from Mojang over TLS, so its signature isn't checked; a malformed token
// just yields "".
func xuid(jwt string) string {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		XUID string `json:"xuid"`
	}
	if json.Unmarshal(raw, &claims) != nil || !digits.MatchString(claims.XUID) {
		return ""
	}
	return claims.XUID
}

var digits = regexp.MustCompile(`^[0-9]{1,20}$`)

// Profile loads the signed-in player. A 404 means the account has no
// Minecraft profile: then the entitlements tell whether it owns the game at
// all (Game Pass accounts can have a profile without store entitlements, so
// ownership is only checked when the profile is missing).
func (c *Client) Profile(ctx context.Context, access string) (Profile, error) {
	var p Profile
	err := c.HTTP.JSON(ctx, http.MethodGet, c.Endpoints.Services+"/minecraft/profile", bearer(access), nil, &p)
	var se *netx.StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		if owned, oerr := c.owns(ctx, access); oerr == nil && !owned {
			return Profile{}, ErrNotOwned
		}
		return Profile{}, ErrNoProfile
	}
	if err != nil {
		return Profile{}, fmt.Errorf("minecraft profile: %w", err)
	}
	return p.valid()
}

func (c *Client) owns(ctx context.Context, access string) (bool, error) {
	var out struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := c.HTTP.JSON(ctx, http.MethodGet, c.Endpoints.Services+"/entitlements/mcstore", bearer(access), nil, &out); err != nil {
		return false, err
	}
	return len(out.Items) > 0, nil
}

var (
	uuidHex    = regexp.MustCompile(`^[0-9a-f]{32}$`)
	playerName = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)
)

// valid rejects a profile whose ID or name we couldn't safely use as a file
// name, argument or account key.
func (p Profile) valid() (Profile, error) {
	p.ID = strings.ToLower(strings.ReplaceAll(p.ID, "-", ""))
	if !uuidHex.MatchString(p.ID) || !playerName.MatchString(p.Name) {
		return Profile{}, errors.New("mcapi: malformed profile")
	}
	return p, nil
}

// UploadSkin replaces the worn skin with png (already validated and
// re-encoded by the caller).
func (c *Client) UploadSkin(ctx context.Context, access string, slim bool, png []byte) (Profile, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	variant := "classic"
	if slim {
		variant = "slim"
	}
	_ = mw.WriteField("variant", variant)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="skin.png"`)
	h.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(h)
	if err != nil {
		return Profile{}, err
	}
	_, _ = part.Write(png)
	if err := mw.Close(); err != nil {
		return Profile{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoints.Services+"/minecraft/profile/skins", &body)
	if err != nil {
		return Profile{}, err
	}
	req.Header = bearer(access)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var p Profile
	if err := c.HTTP.DoJSON(req, &p); err != nil {
		return Profile{}, fmt.Errorf("upload skin: %w", err)
	}
	return p.valid()
}

// ResetSkin goes back to the default skin.
func (c *Client) ResetSkin(ctx context.Context, access string) (Profile, error) {
	return c.change(ctx, access, http.MethodDelete, "/minecraft/profile/skins/active", nil)
}

// SetCape wears an owned cape.
func (c *Client) SetCape(ctx context.Context, access, id string) (Profile, error) {
	return c.change(ctx, access, http.MethodPut, "/minecraft/profile/capes/active", map[string]string{"capeId": id})
}

// HideCape wears no cape.
func (c *Client) HideCape(ctx context.Context, access string) (Profile, error) {
	return c.change(ctx, access, http.MethodDelete, "/minecraft/profile/capes/active", nil)
}

func (c *Client) change(ctx context.Context, access, method, path string, in any) (Profile, error) {
	var p Profile
	if err := c.HTTP.JSON(ctx, method, c.Endpoints.Services+path, bearer(access), in, &p); err != nil {
		return Profile{}, fmt.Errorf("profile change: %w", err)
	}
	return p.valid()
}

// LookupPlayer finds another player's skin and cape by name.
func (c *Client) LookupPlayer(ctx context.Context, name string) (Player, error) {
	if !playerName.MatchString(name) {
		return Player{}, ErrNoPlayer
	}
	var id struct {
		ID string `json:"id"`
	}
	err := c.HTTP.JSON(ctx, http.MethodGet, c.Endpoints.Mojang+"/users/profiles/minecraft/"+url.PathEscape(name), nil, nil, &id)
	var se *netx.StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound || err == nil && id.ID == "" {
		return Player{}, ErrNoPlayer
	}
	if err != nil {
		return Player{}, fmt.Errorf("player lookup: %w", err)
	}
	if !uuidHex.MatchString(id.ID) {
		return Player{}, errors.New("mcapi: malformed player id")
	}
	var prof struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Properties []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"properties"`
	}
	if err := c.HTTP.JSON(ctx, http.MethodGet, c.Endpoints.Session+"/session/minecraft/profile/"+id.ID, nil, nil, &prof); err != nil {
		return Player{}, fmt.Errorf("player profile: %w", err)
	}
	pl := Player{ID: id.ID, Name: prof.Name}
	for _, p := range prof.Properties {
		if p.Name != "textures" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(p.Value)
		if err != nil {
			return Player{}, errors.New("mcapi: malformed textures")
		}
		var tex struct {
			Textures struct {
				Skin struct {
					URL      string `json:"url"`
					Metadata struct {
						Model string `json:"model"`
					} `json:"metadata"`
				} `json:"SKIN"`
				Cape struct {
					URL string `json:"url"`
				} `json:"CAPE"`
			} `json:"textures"`
		}
		if json.Unmarshal(raw, &tex) != nil {
			return Player{}, errors.New("mcapi: malformed textures")
		}
		pl.SkinURL, pl.Slim, pl.CapeURL = tex.Textures.Skin.URL, tex.Textures.Skin.Metadata.Model == "slim", tex.Textures.Cape.URL
	}
	return pl, nil
}

// textureHost serves every skin and cape. Profiles still hand out http://
// links, so TextureHash accepts both schemes and TextureURL always uses https.
const textureHost = "textures.minecraft.net"

var textureHash = regexp.MustCompile(`^[0-9a-f]{16,128}$`)

// TextureHash returns the hash naming a texture URL, or ErrTexture for
// anything that isn't a Minecraft texture.
func TextureHash(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host != textureHost || u.User != nil || u.RawQuery != "" {
		return "", ErrTexture
	}
	hash, ok := strings.CutPrefix(u.Path, "/texture/")
	if !ok || !textureHash.MatchString(hash) {
		return "", ErrTexture
	}
	return hash, nil
}

// TextureURL is the https download URL of a texture hash.
func TextureURL(hash string) string { return "https://" + textureHost + "/texture/" + hash }
