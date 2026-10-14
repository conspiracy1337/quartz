// Package authtest is a fake Microsoft, Xbox Live and Minecraft services
// server on one local port, for tests and for the `fakeauth` development
// build. It answers the browser, device code and refresh flows, the profile,
// skin and cape calls. Fields can be changed between calls (under Lock).
package authtest

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"quartz/internal/auth"
	"quartz/internal/auth/msa"
	"quartz/internal/auth/xbox"
	"quartz/internal/core/netx"
	"quartz/internal/mcapi"
)

// XUID is the Xbox user ID the fake Minecraft token carries.
const XUID = "2535400000000001"

// Server is the fake. Lock it while changing fields from another goroutine.
type Server struct {
	*httptest.Server
	sync.Mutex

	Profile      mcapi.Profile // ID "" means the account has no profile
	Owns         bool          // entitlements, asked only without a profile
	XErr         int64         // non-zero refuses XSTS with this code
	Revoked      bool          // refresh tokens answer invalid_grant
	Down         bool          // every endpoint answers 503
	PendingPolls int           // device polls answered "pending" before success
	ExpiredCodes int           // device codes that expire before the user enters them
	Uploads      int           // skins uploaded

	n     int // token counter
	codes int // device codes handed out
}

// DefaultProfile is the player the fake signs in as.
func DefaultProfile() mcapi.Profile {
	return mcapi.Profile{
		ID: "069a79f444e94726a5befca90e38aaf5", Name: "Notch",
		Capes: []mcapi.Cape{{ID: "cape-1", State: "INACTIVE", Alias: "Test cape", URL: "http://textures.minecraft.net/texture/2340c0e03dd24a11b15a8b33c2a7e9e32abb2051b2481d0ba7defd635ca7a933"}},
	}
}

// New starts the fake.
func New() *Server {
	s := &Server{Profile: DefaultProfile(), Owns: true}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// Service returns an auth service pointed at the fake. Public player lookups
// (api.mojang.com, the session server) stay live: they need no sign-in.
func (s *Server) Service(h *netx.Client, clientID string) *auth.Service {
	svc := auth.New(h, clientID)
	svc.MSA.Endpoints = msa.Endpoints{Authorize: s.URL + "/oauth/authorize", Token: s.URL + "/oauth/token", DeviceCode: s.URL + "/oauth/devicecode"}
	svc.Xbox.Endpoints = xbox.Endpoints{User: s.URL + "/xbox/user", XSTS: s.URL + "/xbox/xsts"}
	svc.MC.Endpoints.Services = s.URL + "/mc"
	return svc
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.Lock()
	defer s.Unlock()
	if s.Down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /oauth/authorize":
		q := r.URL.Query()
		redirect, err := url.Parse(q.Get("redirect_uri"))
		if err != nil || redirect.Hostname() != "localhost" {
			http.Error(w, "bad redirect", http.StatusBadRequest)
			return
		}
		redirect.RawQuery = url.Values{"code": {"fake-code"}, "state": {q.Get("state")}}.Encode()
		http.Redirect(w, r, redirect.String(), http.StatusFound) //nolint:gosec // only ever to localhost, checked above
	case "POST /oauth/devicecode":
		s.codes++
		writeJSON(w, map[string]any{"device_code": "fake-device", "user_code": fmt.Sprintf("QUARTZ%d", 41+s.codes), "verification_uri": "https://www.microsoft.com/link", "expires_in": 900, "interval": 1})
	case "POST /oauth/token":
		s.token(w, r)
	case "POST /xbox/user":
		writeJSON(w, map[string]any{"Token": "fake-xbl", "DisplayClaims": map[string]any{"xui": []any{map[string]string{"uhs": "fake-uhs"}}}})
	case "POST /xbox/xsts":
		if s.XErr != 0 {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]any{"XErr": s.XErr})
			return
		}
		writeJSON(w, map[string]any{"Token": "fake-xsts", "DisplayClaims": map[string]any{"xui": []any{map[string]string{"uhs": "fake-uhs"}}}})
	case "POST /mc/authentication/login_with_xbox":
		s.n++
		claims := base64.RawURLEncoding.EncodeToString([]byte(`{"xuid":"` + XUID + `"}`))
		writeJSON(w, map[string]any{"access_token": fmt.Sprintf("eyJmake.%s.mc-access-%d", claims, s.n), "expires_in": 86400})
	case "GET /mc/minecraft/profile":
		if s.Profile.ID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, s.Profile)
	case "GET /mc/entitlements/mcstore":
		items := []any{}
		if s.Owns {
			items = append(items, map[string]string{"name": "game_minecraft"})
		}
		writeJSON(w, map[string]any{"items": items})
	case "POST /mc/minecraft/profile/skins":
		s.upload(w, r)
	case "DELETE /mc/minecraft/profile/skins/active":
		// Mojang puts one of its default skins on instead.
		s.Profile.Skins = []mcapi.Skin{{ID: "default", State: "ACTIVE", Variant: "CLASSIC", URL: "http://textures.minecraft.net/texture/" + DefaultSkinTexture}}
		writeJSON(w, s.Profile)
	case "PUT /mc/minecraft/profile/capes/active":
		var in struct {
			CapeID string `json:"capeId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		s.setCape(in.CapeID)
		writeJSON(w, s.Profile)
	case "DELETE /mc/minecraft/profile/capes/active":
		s.setCape("")
		writeJSON(w, s.Profile)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	switch r.PostForm.Get("grant_type") {
	case "refresh_token":
		if s.Revoked || !strings.HasPrefix(r.PostForm.Get("refresh_token"), "fake-refresh-") {
			oauthError(w, "invalid_grant")
			return
		}
	case "urn:ietf:params:oauth:grant-type:device_code":
		if s.ExpiredCodes > 0 {
			s.ExpiredCodes--
			oauthError(w, "expired_token")
			return
		}
		if s.PendingPolls > 0 {
			s.PendingPolls--
			oauthError(w, "authorization_pending")
			return
		}
	case "authorization_code":
		if r.PostForm.Get("code") != "fake-code" || r.PostForm.Get("code_verifier") == "" {
			oauthError(w, "invalid_grant")
			return
		}
	default:
		oauthError(w, "unsupported_grant_type")
		return
	}
	s.n++
	writeJSON(w, map[string]any{"access_token": fmt.Sprintf("fake-msa-%d", s.n), "refresh_token": fmt.Sprintf("fake-refresh-%d", s.n), "expires_in": 3600})
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // the body is capped by MaxBytesReader above
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file", http.StatusBadRequest)
		return
	}
	data, _ := io.ReadAll(f)
	sum := sha256.Sum256(data)
	variant := "CLASSIC"
	if r.FormValue("variant") == "slim" {
		variant = "SLIM"
	}
	s.Uploads++
	s.Profile.Skins = []mcapi.Skin{{ID: "skin", State: "ACTIVE", Variant: variant, URL: "http://textures.minecraft.net/texture/" + hex.EncodeToString(sum[:])}}
	writeJSON(w, s.Profile)
}

func (s *Server) setCape(id string) {
	for i := range s.Profile.Capes {
		s.Profile.Capes[i].State = "INACTIVE"
		if s.Profile.Capes[i].ID == id {
			s.Profile.Capes[i].State = "ACTIVE"
		}
	}
}

// DefaultSkinTexture is the texture hash a reset skin gets; tests put a skin
// at that name in the texture cache so nothing is downloaded.
const DefaultSkinTexture = "d5c4ee5ce20aed9e33e866c66caa37178606234b3721084bf01d13320fb2eb3f"

func oauthError(w http.ResponseWriter, code string) {
	w.WriteHeader(http.StatusBadRequest)
	writeJSON(w, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
