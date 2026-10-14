package skins

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"quartz/internal/imagesrv"
	"strings"
	"testing"

	"quartz/internal/core/netx"
	"quartz/internal/mcapi"
)

var discard = slog.New(slog.DiscardHandler)

// skinPNG draws a test skin. slim leaves the classic-only arm column empty.
func skinPNG(t *testing.T, w, h int, slim bool, seed uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: seed, A: 255}) //nolint:gosec // test pattern
		}
	}
	if slim {
		for y := 20; y < 32; y++ {
			img.SetNRGBA(54, y, color.NRGBA{R: 9, A: 0}) // transparent, with colour noise
			img.SetNRGBA(55, y, color.NRGBA{})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCanonicalize(t *testing.T) {
	classic, err := Canonicalize(skinPNG(t, 64, 64, false, 1))
	if err != nil || classic.Slim || len(classic.Hash) != 64 {
		t.Fatalf("classic: %+v %v", classic.Slim, err)
	}
	slim, err := Canonicalize(skinPNG(t, 64, 64, true, 1))
	if err != nil || !slim.Slim {
		t.Fatalf("slim: %v", err)
	}
	// Trailing bytes after IEND don't survive the re-encode.
	withTail, err := Canonicalize(append(skinPNG(t, 64, 64, false, 1), []byte("<script>")...))
	if err != nil || withTail.Hash != classic.Hash || bytes.Contains(withTail.PNG, []byte("<script>")) {
		t.Fatalf("trailing data: %v", err)
	}
	// Legacy 64×32 skins become 64×64 with mirrored left limbs.
	legacy, err := Canonicalize(skinPNG(t, 64, 32, false, 2))
	if err != nil {
		t.Fatal(err)
	}
	img, _ := png.Decode(bytes.NewReader(legacy.PNG))
	if img.Bounds().Dy() != 64 {
		t.Fatalf("legacy height %d", img.Bounds().Dy())
	}
	// Front of the right leg (4..7, 20) mirrors into the left leg's front (20..23, 52).
	src := img.At(4, 20).(color.NRGBA)
	dst := img.At(23, 52).(color.NRGBA)
	if src != dst {
		t.Fatalf("legacy mirror: %v vs %v", src, dst)
	}
	for _, bad := range [][]byte{
		[]byte("not a png"),
		skinPNG(t, 128, 128, false, 1),
		skinPNG(t, 64, 48, false, 1),
		bytes.Repeat([]byte{0}, MaxSize+1),
	} {
		if _, err := Canonicalize(bad); !errors.Is(err, ErrNotSkin) {
			t.Fatalf("accepted bad input (%d bytes): %v", len(bad), err)
		}
	}
}

func openLib(t *testing.T, srv *httptest.Server) *Library {
	t.Helper()
	h, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	mc := mcapi.New(h)
	if srv != nil {
		mc.Endpoints = mcapi.Endpoints{Mojang: srv.URL, Session: srv.URL}
	}
	dir := t.TempDir()
	lib, err := Open(filepath.Join(dir, "skins"), h, mc, NewTextures(filepath.Join(dir, "tex"), h), discard)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestLibrary(t *testing.T) {
	lib := openLib(t, nil)
	file := filepath.Join(t.TempDir(), "My Skin.png")
	if err := os.WriteFile(file, skinPNG(t, 64, 64, true, 3), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := lib.ImportFile(file)
	if err != nil || s.Name != "My Skin" || !s.Slim || s.Source != "file" {
		t.Fatalf("import: %+v %v", s, err)
	}
	again, _ := lib.Add(skinPNG(t, 64, 64, true, 3), "Other", "link", nil)
	if again.Name != "My Skin" || len(lib.List()) != 1 {
		t.Fatalf("dedup: %+v", again)
	}
	if err := lib.Rename(s.Hash, "  Renamed  "); err != nil {
		t.Fatal(err)
	}
	if err := lib.Rename(s.Hash, strings.Repeat("x", 41)); !errors.Is(err, ErrName) {
		t.Fatalf("long name: %v", err)
	}
	if err := lib.SetSlim(s.Hash, false); err != nil {
		t.Fatal(err)
	}
	removed, err := lib.Remove(s.Hash)
	if err != nil || removed.Name != "Renamed" || removed.Slim || len(lib.List()) != 0 {
		t.Fatalf("remove: %+v %v", removed, err)
	}
	if err := lib.Restore(removed); err != nil || len(lib.List()) != 1 {
		t.Fatalf("restore: %v", err)
	}
	if _, err := lib.Path("../../etc/passwd"); !errors.Is(err, ErrUnknown) {
		t.Fatal("path accepted a non-hash")
	}
}

func TestImportFromLink(t *testing.T) {
	skin := skinPNG(t, 64, 64, false, 4)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cool_skin.png":
			_, _ = w.Write(skin)
		case "/huge.png":
			_, _ = w.Write(bytes.Repeat([]byte{1}, MaxSize+10))
		}
	}))
	defer srv.Close()
	lib := openLib(t, nil)
	lib.http, _ = netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{TLS: srv.Client().Transport.(*http.Transport).TLSClientConfig})
	s, err := lib.ImportURL(context.Background(), srv.URL+"/cool_skin.png")
	if err != nil || s.Name != "cool_skin" || s.Source != "link" {
		t.Fatalf("link: %+v %v", s, err)
	}
	if _, err := lib.ImportURL(context.Background(), srv.URL+"/huge.png"); !errors.Is(err, ErrNotSkin) {
		t.Fatalf("huge: %v", err)
	}
	if _, err := lib.ImportURL(context.Background(), "http://example.com/a.png"); err == nil {
		t.Fatal("plain http accepted")
	}
}

func TestHandlerServesOnlyHexNames(t *testing.T) {
	lib := openLib(t, nil)
	s, err := lib.Add(skinPNG(t, 64, 64, false, 5), "x", "file", nil)
	if err != nil {
		t.Fatal(err)
	}
	h := imagesrv.Handler(map[string]imagesrv.Resolver{
		"skin": func(_ context.Context, n string) (string, error) { return lib.Path(n) },
		"mc":   lib.tex.Path,
	})
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	if rec := get("/tex/skin/" + s.Hash + ".png"); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("skin: %d", rec.Code)
	}
	for _, bad := range []string{
		"/tex/skin/../index.json",
		"/tex/skin/" + s.Hash,
		"/tex/skin/" + strings.ToUpper(s.Hash) + ".png",
		"/tex/other/" + s.Hash + ".png",
		"/tex/mc/..%2f..%2fetc.png",
		"/index.json",
	} {
		if rec := get(bad); rec.Code != 404 {
			t.Fatalf("%s: %d", bad, rec.Code)
		}
	}
}

func TestDefaults(t *testing.T) {
	// Indexes from jshell: Math.floorMod(UUID.fromString(u).hashCode(), 18).
	for id, want := range map[string]Default{
		"069a79f444e94726a5befca90e38aaf5": {Key: "slim-alex", Slim: true},
		"853c80ef3c3749fdaa49938b674adae6": {Key: "slim-ari", Slim: true},
		"5627dd98e6be3c21b8a8e92344183641": {Key: "wide-efe"},
		"not-a-uuid":                       {Key: "slim-alex", Slim: true},
	} {
		if got := DefaultFor(id); got != want {
			t.Errorf("DefaultFor(%s) = %+v, want %+v", id, got, want)
		}
	}
	if Steve != (Default{Key: "wide-steve"}) {
		t.Errorf("Steve = %+v", Steve)
	}
	tex := NewTextures(t.TempDir(), nil)
	for _, n := range defaultNames {
		for _, v := range []string{"slim", "wide"} {
			p, err := tex.DefaultPath(v + "-" + n)
			if err != nil {
				t.Fatal(err)
			}
			if data, _ := os.ReadFile(p); !isPNG(data) { //nolint:gosec // test temp dir
				t.Errorf("%s-%s is not a PNG", v, n)
			}
		}
	}
	for _, bad := range []string{"", "steve", "wide-", "wide-../x", "tall-steve", "wide-steve/../../x"} {
		if _, err := tex.DefaultPath(bad); err == nil {
			t.Errorf("DefaultPath(%q) accepted", bad)
		}
	}
}
