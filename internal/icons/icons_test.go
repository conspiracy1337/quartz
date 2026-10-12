package icons

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAddAndPath(t *testing.T) {
	l := Open(t.TempDir())
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 400, 300)))
	hash, err := l.Add(buf.Bytes())
	if err != nil || !ValidHash(hash) {
		t.Fatalf("add = %q, %v", hash, err)
	}
	p, err := l.Path(hash)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(p)
	cfg, _ := png.DecodeConfig(f)
	_ = f.Close()
	if cfg.Width != Side || cfg.Height != Side {
		t.Fatalf("stored %dx%d", cfg.Width, cfg.Height)
	}
	for _, bad := range []string{"../icons", hash[:63], filepath.Join("..", hash)} {
		if _, err := l.Path(bad); err == nil {
			t.Errorf("path accepted %q", bad)
		}
	}
	if _, err := l.Add([]byte("not an image")); !errors.Is(err, ErrNotIcon) {
		t.Fatal("non-image accepted")
	}
}
