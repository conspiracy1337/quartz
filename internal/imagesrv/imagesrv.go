// Package imagesrv serves the launcher's own images to the UI, same-origin,
// so the page's CSP needs no extra host:
//
//	/tex/skin/<sha256>.png  a library skin
//	/tex/mc/<hash>.png      a textures.minecraft.net texture (fetched once)
//	/tex/icon/<sha256>.png  an instance icon
//
// Each kind resolves a name to a file; resolvers accept only hex names, so a
// request can't reach any other file.
package imagesrv

import (
	"context"
	"net/http"
	"strings"
)

// Resolver turns an image name into its file.
type Resolver func(ctx context.Context, name string) (string, error)

// Handler serves GET /tex/<kind>/<name>.png from routes.
func Handler(routes map[string]Resolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		kind, file, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/tex/"), "/")
		name, isPNG := strings.CutSuffix(file, ".png")
		resolve, known := routes[kind]
		if !ok || !isPNG || !known || !strings.HasPrefix(r.URL.Path, "/tex/") {
			http.NotFound(w, r)
			return
		}
		path, err := resolve(r.Context(), name)
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
