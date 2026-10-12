package skins

import (
	"net/http"
	"strings"
)

// Handler serves skin and cape images to the UI, same-origin, so the page's
// CSP needs no extra host:
//
//	/tex/skin/<sha256>.png  a library skin
//	/tex/mc/<hash>.png      a textures.minecraft.net texture (fetched once)
//
// Only hex names are accepted, so a request can't reach any other file.
func Handler(lib *Library, tex *Textures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		kind, file, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/tex/"), "/")
		hash, isPNG := strings.CutSuffix(file, ".png")
		if !ok || !isPNG || !strings.HasPrefix(r.URL.Path, "/tex/") {
			http.NotFound(w, r)
			return
		}
		var (
			path string
			err  error
		)
		switch kind {
		case "skin":
			path, err = lib.Path(hash)
		case "mc":
			path, err = tex.Path(r.Context(), hash)
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "image/png")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "private, max-age=31536000, immutable") // content-addressed
		http.ServeFile(w, r, path)
	})
}
