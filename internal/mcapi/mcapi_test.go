package mcapi

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quartz/internal/core/netx"
)

const profileJSON = `{"id":"069A79F444E94726A5BEFCA90E38AAF5","name":"Notch",
 "skins":[{"id":"s1","state":"ACTIVE","url":"http://textures.minecraft.net/texture/abc123abc123abc123","variant":"SLIM"}],
 "capes":[{"id":"c1","state":"ACTIVE","url":"http://textures.minecraft.net/texture/def456def456def456","alias":"Migrator"}]}`

func fake(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	hc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := New(hc)
	c.Endpoints = Endpoints{Services: srv.URL, Mojang: srv.URL + "/mojang", Session: srv.URL + "/session"}
	return c
}

func TestLoginAndProfile(t *testing.T) {
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"xuid":"2535405290"}`))
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authentication/login_with_xbox":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"XBL3.0 x=UHS;XSTS"`) {
				t.Errorf("identity token: %s", body)
			}
			_, _ = io.WriteString(w, `{"access_token":"h.`+claims+`.s","expires_in":86400}`)
		case "/minecraft/profile":
			if r.Header.Get("Authorization") != "Bearer TOKEN" {
				t.Errorf("auth header %q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, profileJSON)
		}
	})
	tok, err := c.LoginWithXbox(context.Background(), "UHS", "XSTS")
	if err != nil || tok.XUID != "2535405290" {
		t.Fatalf("login %+v %v", tok, err)
	}
	p, err := c.Profile(context.Background(), "TOKEN")
	if err != nil || p.ID != "069a79f444e94726a5befca90e38aaf5" || p.Name != "Notch" {
		t.Fatalf("profile %+v %v", p, err)
	}
	if s, ok := p.ActiveSkin(); !ok || s.Variant != "SLIM" {
		t.Fatalf("active skin %+v", s)
	}
}

func TestMissingProfile(t *testing.T) {
	for items, want := range map[string]error{`[]`: ErrNotOwned, `[{"name":"game_minecraft"}]`: ErrNoProfile} {
		c := fake(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/entitlements/mcstore" {
				_, _ = io.WriteString(w, `{"items":`+items+`}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"NOT_FOUND"}`)
		})
		if _, err := c.Profile(context.Background(), "T"); !errors.Is(err, want) {
			t.Fatalf("items %s: %v", items, err)
		}
	}
	// A profile we couldn't safely use is refused.
	c := fake(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"id":"../x","name":"a b"}`) })
	if _, err := c.Profile(context.Background(), "T"); err == nil {
		t.Fatal("malformed profile accepted")
	}
}

func TestSkinAndCapeChanges(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /minecraft/profile/skins":
			if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("variant") != "slim" {
				t.Errorf("multipart: %v %v", err, r.MultipartForm)
			}
			f, h, err := r.FormFile("file")
			if err != nil || h.Header.Get("Content-Type") != "image/png" {
				t.Errorf("file part: %v", err)
			} else {
				b, _ := io.ReadAll(f)
				if string(b) != "PNGDATA" {
					t.Errorf("file body %q", b)
				}
			}
		case "DELETE /minecraft/profile/skins/active", "DELETE /minecraft/profile/capes/active":
		case "PUT /minecraft/profile/capes/active":
			b, _ := io.ReadAll(r.Body)
			if string(b) != `{"capeId":"c1"}` {
				t.Errorf("cape body %s", b)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, profileJSON)
	})
	ctx := context.Background()
	if _, err := c.UploadSkin(ctx, "T", true, []byte("PNGDATA")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResetSkin(ctx, "T"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetCape(ctx, "T", "c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HideCape(ctx, "T"); err != nil {
		t.Fatal(err)
	}
}

func TestLookupPlayer(t *testing.T) {
	tex := base64.StdEncoding.EncodeToString([]byte(`{"textures":{"SKIN":{"url":"http://textures.minecraft.net/texture/aaaabbbbccccdddd","metadata":{"model":"slim"}}}}`))
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mojang/users/profiles/minecraft/Notch":
			_, _ = io.WriteString(w, `{"id":"069a79f444e94726a5befca90e38aaf5","name":"Notch"}`)
		case "/mojang/users/profiles/minecraft/Gone":
			w.WriteHeader(http.StatusNoContent)
		case "/session/session/minecraft/profile/069a79f444e94726a5befca90e38aaf5":
			_, _ = io.WriteString(w, `{"id":"069a79f444e94726a5befca90e38aaf5","name":"Notch","properties":[{"name":"textures","value":"`+tex+`"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	p, err := c.LookupPlayer(context.Background(), "Notch")
	if err != nil || !p.Slim || p.SkinURL == "" {
		t.Fatalf("%+v %v", p, err)
	}
	for _, name := range []string{"Gone", "Missing", "../etc", "way_too_long_name_here"} {
		if _, err := c.LookupPlayer(context.Background(), name); !errors.Is(err, ErrNoPlayer) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestTextureHash(t *testing.T) {
	ok := map[string]string{
		"http://textures.minecraft.net/texture/abc123abc123abc1":  "abc123abc123abc1",
		"https://textures.minecraft.net/texture/abc123abc123abc1": "abc123abc123abc1",
	}
	for in, want := range ok {
		if got, err := TextureHash(in); err != nil || got != want {
			t.Fatalf("%s: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{
		"https://evil.example/texture/abc123abc123abc1",
		"https://textures.minecraft.net.evil.example/texture/abc123abc123abc1",
		"https://textures.minecraft.net/texture/../../etc/passwd",
		"https://textures.minecraft.net/texture/ABC123ABC123ABC1",
		"https://user@textures.minecraft.net/texture/abc123abc123abc1",
		"file:///textures.minecraft.net/texture/abc123abc123abc1",
	} {
		if _, err := TextureHash(bad); !errors.Is(err, ErrTexture) {
			t.Fatalf("%s accepted", bad)
		}
	}
	if TextureURL("abc") != "https://textures.minecraft.net/texture/abc" {
		t.Fatal("texture url")
	}
}
