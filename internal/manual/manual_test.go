package manual

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // test fixtures
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/modplatform"
)

type cdn map[string][]byte

func (c cdn) Do(req *http.Request) (*http.Response, error) {
	body, ok := c[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Header: http.Header{}, Request: req}, nil
}

// twins is a Modrinth that knows some files by sha1.
type twins struct {
	modplatform.Provider
	files map[string]modplatform.File
}

func (t twins) Identify(_ context.Context, hs []modplatform.Hashes) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	for _, h := range hs {
		if f, ok := t.files[h.SHA1]; ok {
			out[h.SHA1] = modplatform.Version{File: f}
		}
	}
	return out, nil
}

func sum(b []byte) string {
	s := sha1.Sum(b) //nolint:gosec // test fixtures
	return hex.EncodeToString(s[:])
}

type env struct {
	t      *testing.T
	svc    *Service
	folder string
}

func newEnv(t *testing.T, files cdn, mr map[string]modplatform.File) *env {
	t.Helper()
	root := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	st, err := store.Open(filepath.Join(root, "store", "content"), nil, log)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := download.OpenVerified(filepath.Join(root, "verified.json"), root, log)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(root, "Downloads")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	svc := New(Deps{Store: st, Download: download.New(files, ver), Modrinth: twins{files: mr}, Folder: folder, Log: log})
	return &env{t: t, svc: svc, folder: folder}
}

// waitDone runs the set's step and reports when it returns.
func (e *env) run(set *Set) (<-chan error, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- set.wait(ctx) }()
	return done, cancel
}

func (e *env) state(job, id string) State {
	v, err := e.svc.View(job)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, it := range v.Items {
		if it.ID == id {
			return it.State
		}
	}
	return ""
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestWatchedDownloadsFolderMatchesBySizeAndHash(t *testing.T) {
	e := newEnv(t, nil, nil)
	balm, waystones := []byte("balm jar bytes"), []byte("waystones jar")
	set := e.svc.Register([]Need{
		{Name: "Balm", FileName: "balm.jar", Size: int64(len(balm)), SHA1: sum(balm), Page: "https://www.curseforge.com/minecraft/mc-mods/balm/download/1"},
		{Name: "Waystones", FileName: "waystones.jar", Size: int64(len(waystones)), SHA1: sum(waystones)},
	})
	set.Bind("job1")
	// Downloaded before the job asked: picked up by the first sweep.
	if err := os.WriteFile(filepath.Join(e.folder, "waystones.jar"), waystones, 0o600); err != nil {
		t.Fatal(err)
	}
	done, cancel := e.run(set)
	defer cancel()
	eventually(t, "waystones found", func() bool { return e.state("job1", "1") == Found })
	// A partial download, and a file of the right size but other bytes, are ignored.
	_ = os.WriteFile(filepath.Join(e.folder, "balm.jar.part"), balm, 0o600)
	_ = os.WriteFile(filepath.Join(e.folder, "other.jar"), bytes.Repeat([]byte("x"), len(balm)), 0o600)
	time.Sleep(2 * settle)
	if e.state("job1", "0") != Waiting {
		t.Fatal("matched the wrong file")
	}
	if pages, _ := e.svc.Pages("job1"); len(pages) != 1 {
		t.Fatalf("pages %v", pages)
	}
	_ = os.WriteFile(filepath.Join(e.folder, "balm.jar"), balm, 0o600)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the step never ended")
	}
	if _, ok := e.svc.d.Store.Blob(sum(balm)); !ok {
		t.Fatal("the found file is not in the store")
	}
	if _, err := os.Stat(filepath.Join(e.folder, "balm.jar")); err != nil {
		t.Fatal("the player's file was moved")
	}
}

func TestSkipOfferAndModrinthCopy(t *testing.T) {
	controlling, balm := []byte("controlling"), []byte("balm")
	url := "https://cdn.modrinth.com/data/B/balm.jar"
	e := newEnv(t, cdn{url: balm}, map[string]modplatform.File{sum(balm): {URL: url, SHA1: sum(balm), Size: int64(len(balm))}})
	e.svc.d.Folder = "" // nothing watched: drops and buttons only
	set := e.svc.Register([]Need{
		{Name: "Balm", Size: int64(len(balm)), SHA1: sum(balm)},
		{Name: "Controlling", Size: int64(len(controlling)), SHA1: sum(controlling)},
		{Name: "Clumps", Size: 5, SHA1: strings.Repeat("a", 40)},
	})
	set.Bind("job2")
	done, cancel := e.run(set)
	defer cancel()
	eventually(t, "the Modrinth twin", func() bool {
		v, _ := e.svc.View("job2")
		return v.Items[0].Twin && !v.Items[1].Twin
	})
	if err := e.svc.UseModrinth("job2", "1"); !errors.Is(err, ErrNoTwin) {
		t.Fatalf("no twin: %v", err)
	}
	if err := e.svc.UseModrinth("job2", "0"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the Modrinth copy", func() bool { return e.state("job2", "0") == Found })
	dropped := filepath.Join(t.TempDir(), "Controlling.jar")
	_ = os.WriteFile(dropped, controlling, 0o600)
	if _, err := e.svc.Offer("job2", []string{"relative.jar"}); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("relative path: %v", err)
	}
	if n, err := e.svc.Offer("job2", []string{dropped}); err != nil || n != 1 {
		t.Fatalf("drop: %d %v", n, err)
	}
	if err := e.svc.Skip("job2"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if names := set.SkippedNames(); len(names) != 1 || names[0] != "Clumps" || !set.Skipped(strings.Repeat("a", 40)) {
		t.Fatalf("skipped %v", names)
	}
	if _, err := e.svc.View("nope"); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
}

func TestLabelAndFolder(t *testing.T) {
	if Label(1, "mod") != "1 mod needs a manual download" || Label(3, "file") != "3 files need a manual download" {
		t.Fatal(Label(3, "file"))
	}
	home := t.TempDir()
	cfg := filepath.Join(home, ".config")
	_ = os.MkdirAll(cfg, 0o700)
	_ = os.WriteFile(filepath.Join(cfg, "user-dirs.dirs"), []byte("# x\nXDG_DOWNLOAD_DIR=\"$HOME/Téléchargements\"\n"), 0o600)
	if got := xdgDownloads(home, ""); got != filepath.Join(home, "Téléchargements") {
		t.Fatalf("xdg %q", got)
	}
	_ = os.WriteFile(filepath.Join(cfg, "user-dirs.dirs"), []byte("XDG_DOWNLOAD_DIR=\"$HOME/\"\n"), 0o600)
	if got := xdgDownloads(home, ""); got != "" {
		t.Fatalf("unset dir read as %q", got)
	}
}
