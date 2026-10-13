package localimg

import (
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePNG(t *testing.T, p string, w, h int) os.FileInfo {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	fi, _ := os.Stat(p)
	return fi
}

func TestTokensAndThumbs(t *testing.T) {
	dir := t.TempDir()
	r := New(filepath.Join(dir, "thumbs"))
	ctx := context.Background()
	big := filepath.Join(dir, "2025-03-20_21.40.12.png")
	small := filepath.Join(dir, "icon.png")
	tb := r.Register(big, writePNG(t, big, 1280, 720))
	ts := r.Register(small, writePNG(t, small, 64, 64))

	if p, err := r.Path(ctx, tb); err != nil || p != big {
		t.Fatalf("path %q %v", p, err)
	}
	if _, err := r.Path(ctx, "00000000000000000000000000000000"); err == nil {
		t.Fatal("unknown token resolved")
	}
	if _, err := r.Path(ctx, "../etc/passwd"); err == nil {
		t.Fatal("bad token resolved")
	}
	if p, err := r.Thumb(ctx, ts); err != nil || p != small {
		t.Fatalf("small picture should be its own thumb: %q %v", p, err)
	}
	p, err := r.Thumb(ctx, tb)
	if err != nil || filepath.Ext(p) != ".jpg" {
		t.Fatalf("thumb %q %v", p, err)
	}
	f, _ := os.Open(p)
	cfg, err := jpeg.DecodeConfig(f)
	_ = f.Close()
	if err != nil || cfg.Width != ThumbWidth || cfg.Height != 270 {
		t.Fatalf("thumb size %+v %v", cfg, err)
	}
	if again, _ := r.Thumb(ctx, tb); again != p {
		t.Fatal("thumb not reused")
	}

	// A changed file no longer answers to its old token.
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(big, later, later)
	if _, err := r.Path(ctx, tb); err == nil {
		t.Fatal("changed file still served")
	}
	if w, h, err := Dims(small); err != nil || w != 64 || h != 64 {
		t.Fatalf("dims %d×%d %v", w, h, err)
	}
}
