package modpack

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"quartz/internal/core/hashx"
)

// bigPack is an .mrpack past peekFrom: a small index and a large override
// that never needs to be fetched to preview it.
func bigPack(t *testing.T) string {
	t.Helper()
	noise := make([]byte, peekFrom+(1<<20))
	_, _ = rand.Read(noise)
	index := js(map[string]any{"formatVersion": 1, "game": "minecraft", "versionId": "2.0", "name": "Big",
		"dependencies": map[string]string{"minecraft": "1.21.1", "fabric-loader": "0.16.9"},
		"files":        []any{mrFile("mods/a.jar", "https://cdn.modrinth.com/data/x/a.jar", nil)}})
	return zipOf(t, entry{name: "modrinth.index.json", body: index}, entry{name: "overrides/config/a.toml", body: "a=1"},
		entry{name: "overrides/resourcepacks/huge.zip", body: string(noise)})
}

// servePack serves file; ranges reports whether Range requests are honoured.
func servePack(t *testing.T, file string, ranges bool) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile(file)
		if !ranges {
			r.Header.Del("Range")
		}
		cw := &countingWriter{ResponseWriter: w, n: &sent}
		http.ServeContent(cw, r, "pack.mrpack", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	return srv, &sent
}

type countingWriter struct {
	http.ResponseWriter
	n *atomic.Int64
}

func (c *countingWriter) Write(b []byte) (int, error) {
	c.n.Add(int64(len(b)))
	return c.ResponseWriter.Write(b)
}

func rangedReader(t *testing.T, file string) *Reader {
	t.Helper()
	return NewReader(Deps{
		Cache: t.TempDir(),
		Get: func(ctx context.Context, raw string, h http.Header) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
			if err != nil {
				return nil, err
			}
			req.Header = h
			return http.DefaultClient.Do(req)
		},
		// The background download: the real one checks the platform's hash.
		Fetch: func(_ context.Context, _ string, _ int64, _ hashx.Sum, dest string) error {
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			return os.WriteFile(dest, data, 0o600)
		},
	})
}

func TestPreviewReadsALargePackInPlace(t *testing.T) {
	file := bigPack(t)
	info, _ := os.Stat(file)
	srv, sent := servePack(t, file, true)
	r := rangedReader(t, file)
	p, err := r.open(context.Background(), srv.URL, info.Size(), hashx.Sum{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Archive != "" || p.download == nil || p.Name != "Big" {
		t.Fatalf("not previewed in place: archive %q, download %v, name %q", p.Archive, p.download != nil, p.Name)
	}
	if n := sent.Load(); n > 2*rangeBlock {
		t.Fatalf("the preview fetched %d bytes of a %d byte pack", n, info.Size())
	}
	st, err := p.Stats()
	if err != nil || st.Mods != 1 || st.Files != 2 {
		t.Fatalf("stats from the listing = %+v, %v", st, err)
	}
	p.Origin.Project = "keep"
	if err := r.Complete(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if p.Archive == "" || p.download != nil || p.Origin.Project != "keep" {
		t.Fatalf("Complete must re-read the downloaded archive and keep the platform's data: %+v", p)
	}
	r.Discard(p)
}

func TestPreviewFallsBackWithoutRanges(t *testing.T) {
	file := bigPack(t)
	info, _ := os.Stat(file)
	srv, _ := servePack(t, file, false)
	p, err := rangedReader(t, file).open(context.Background(), srv.URL, info.Size(), hashx.Sum{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Archive == "" || p.download != nil {
		t.Fatal("a server without ranges must get the whole pack downloaded")
	}
}

func TestRangeMatches(t *testing.T) {
	for h, want := range map[string]bool{
		"bytes 0-99/1000": true, "bytes 0-99/999": false, "bytes 1-99/1000": false, "0-99/1000": false, "bytes 0-x/1000": false,
	} {
		if rangeMatches(h, 0, 99, 1000) != want {
			t.Errorf("%q: want %v", h, want)
		}
	}
}
