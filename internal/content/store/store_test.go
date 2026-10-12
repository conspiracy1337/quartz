package store

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"quartz/internal/core/hashx"
	"quartz/internal/core/linkx"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "store"), nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o700); err != nil {
		t.Fatal(err)
	}
	return s, game
}

func file(t *testing.T, path, body string) hashx.Multi {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := hashx.MultiFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func links(t *testing.T, p string) uint64 {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink) //nolint:unconvert // Nlink is uint32 on some platforms
	}
	return 0
}

func TestAdoptNewFileBecomesTheBlob(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("link counts")
	}
	s, game := open(t)
	p := filepath.Join(game, "sodium.jar")
	h := file(t, p, "mod bytes")
	m, err := s.Adopt(p, h)
	if err != nil {
		t.Fatal(err)
	}
	if m != linkx.Hardlink || links(t, p) != 2 || !same(p, s.Path(h.SHA1)) {
		t.Fatalf("method %v, links %d", m, links(t, p))
	}
	if info, _ := os.Stat(s.Path(h.SHA1)); info.Mode().Perm()&0o222 != 0 {
		t.Fatal("blob is writable")
	}
	if b, ok := s.Blob(h.SHA1); !ok || b.Size != 9 || b.Murmur2 != h.Murmur2 {
		t.Fatalf("%+v %v", b, ok)
	}
	// A second adoption of the same file is a no-op.
	if m, err := s.Adopt(p, h); err != nil || m != linkx.Hardlink {
		t.Fatalf("%v %v", m, err)
	}
}

func TestAdoptDuplicateIsDeduplicated(t *testing.T) {
	s, game := open(t)
	a, b := filepath.Join(game, "a.jar"), filepath.Join(game, "b.jar")
	h := file(t, a, "same")
	file(t, b, "same")
	if _, err := s.Adopt(a, h); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Adopt(b, h); err != nil {
		t.Fatal(err)
	}
	if !same(a, b) {
		t.Fatal("duplicate not shared")
	}
}

func TestAdoptAcrossVolumesKeepsTheFile(t *testing.T) {
	s, game := open(t)
	p := filepath.Join(game, "x.jar")
	h := file(t, p, "far away")
	// Simulate another volume: hardlinks and clones fail.
	ol, os2 := linkFn, shareFn
	linkFn = func(string, string) error { return errors.New("EXDEV") }
	shareFn = func(string, string) (linkx.Method, error) { return linkx.Copy, linkx.ErrUnsupported }
	t.Cleanup(func() { linkFn, shareFn = ol, os2 })
	m, err := s.Adopt(p, h)
	if err != nil {
		t.Fatal(err)
	}
	if m != linkx.Copy || same(p, s.Path(h.SHA1)) {
		t.Fatalf("method %v", m)
	}
	if _, ok := s.Blob(h.SHA1); !ok {
		t.Fatal("the blob should still be stored for other instances")
	}
	if got, _ := os.ReadFile(p); string(got) != "far away" { //nolint:gosec // test temp dir
		t.Fatal("player's file changed")
	}
}

func TestCollisionIsRefused(t *testing.T) {
	s, game := open(t)
	p := filepath.Join(game, "a.jar")
	h := file(t, p, "one")
	if _, err := s.Adopt(p, h); err != nil {
		t.Fatal(err)
	}
	q := filepath.Join(game, "b.jar")
	fake := file(t, q, "two")
	fake.SHA1 = h.SHA1 // pretend sha1 collided
	if _, err := s.Adopt(q, fake); !errors.Is(err, ErrCollision) {
		t.Fatalf("err = %v", err)
	}
}

func TestIngestNeverLinksTheSource(t *testing.T) {
	s, game := open(t)
	src := filepath.Join(t.TempDir(), "dropped.jar")
	file(t, src, "dropped")
	h, err := s.Ingest(src)
	if err != nil {
		t.Fatal(err)
	}
	if same(src, s.Path(h.SHA1)) {
		t.Fatal("the user's own file was linked into the store")
	}
	dst := filepath.Join(game, "dropped.jar")
	if _, err := s.Place(h.SHA1, dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "dropped" { //nolint:gosec // test temp dir
		t.Fatal("placed content")
	}
	// Ingesting it again is fine.
	if _, err := s.Ingest(src); err != nil {
		t.Fatal(err)
	}
}

func TestSealChecksTheHash(t *testing.T) {
	s, _ := open(t)
	h := hashx.Multi{SHA1: "0123456789012345678901234567890123456789"}
	_ = os.MkdirAll(filepath.Dir(s.Path(h.SHA1)), 0o700)
	_ = os.WriteFile(s.Path(h.SHA1), []byte("not that"), 0o600)
	if _, err := s.Seal(h.SHA1); !errors.Is(err, hashx.ErrMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnlinkResealsTheBlob(t *testing.T) {
	s, game := open(t)
	p := filepath.Join(game, "a.jar")
	h := file(t, p, "x")
	if _, err := s.Adopt(p, h); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlink(p, h.SHA1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("not removed")
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(s.Path(h.SHA1)); info.Mode().Perm()&0o222 != 0 {
			t.Fatal("blob writable after unlink")
		}
	}
}

func TestGC(t *testing.T) {
	s, game := open(t)
	now := time.Unix(1_000_000, 0)
	s.now = func() time.Time { return now }
	keep := file(t, filepath.Join(game, "keep.jar"), "keep")
	drop := file(t, filepath.Join(game, "drop.jar"), "drop")
	held := file(t, filepath.Join(game, "held.jar"), "held")
	for _, n := range []string{"keep.jar", "drop.jar", "held.jar"} {
		h, _ := hashx.MultiFile(filepath.Join(game, n))
		if _, err := s.Adopt(filepath.Join(game, n), h); err != nil {
			t.Fatal(err)
		}
	}
	release := s.Hold(held.SHA1)
	refs := func(yield func(string)) error { yield(keep.SHA1); return nil }
	// First pass only marks.
	if freed, n, _ := s.GC(refs, time.Hour); n != 0 || freed != 0 {
		t.Fatalf("first pass deleted %d", n)
	}
	u, _ := s.Usage(refs)
	if u.Blobs != 3 || u.Unused != 1 || u.UnusedBytes != 4 {
		t.Fatalf("%+v", u)
	}
	now = now.Add(2 * time.Hour)
	release()
	freed, n, err := s.GC(refs, time.Hour)
	if err != nil || n != 1 || freed != 4 {
		t.Fatalf("freed %d n %d err %v", freed, n, err)
	}
	if _, err := os.Stat(s.Path(drop.SHA1)); !os.IsNotExist(err) {
		t.Fatal("unused blob kept")
	}
	if _, ok := s.Blob(keep.SHA1); !ok {
		t.Fatal("referenced blob deleted")
	}
	if _, ok := s.Blob(held.SHA1); !ok {
		t.Fatal("held blob deleted on its first unheld pass")
	}
}

func TestCleanDeletesUnusedNow(t *testing.T) {
	s, game := open(t)
	now := time.Unix(1_000_000, 0)
	s.now = func() time.Time { return now }
	old := file(t, filepath.Join(game, "old.jar"), "old")
	if _, err := s.Adopt(filepath.Join(game, "old.jar"), old); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	fresh := file(t, filepath.Join(game, "fresh.jar"), "fresh")
	if _, err := s.Adopt(filepath.Join(game, "fresh.jar"), fresh); err != nil {
		t.Fatal(err)
	}
	none := func(func(string)) error { return nil }
	if _, n, err := s.Clean(none, 10*time.Minute); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, ok := s.Blob(fresh.SHA1); !ok {
		t.Fatal("a blob added moments ago was cleaned")
	}
}
