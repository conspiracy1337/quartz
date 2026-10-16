package worlds

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // CurseForge names files by sha1
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/core/download"
	"quartz/internal/core/tasks"
	"quartz/internal/modplatform"
)

// cdn serves fixed bodies by URL.
type cdn map[string][]byte

func (c cdn) Do(req *http.Request) (*http.Response, error) {
	body, ok := c[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Request: req}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Header: http.Header{}, Request: req}, nil
}

func TestInstallDownloadsAndImports(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	ver, err := download.OpenVerified(filepath.Join(root, "verified.json"), root, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	// A CurseForge world zip: one folder holding level.dat.
	src := filepath.Join(t.TempDir(), "Skyblock")
	level(t, src, "Skyblock", 0, false, 1)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, rel := range []string{"level.dat", "region/r.0.0.mca"} {
		w, _ := zw.Create("Skyblock/" + rel)
		b, _ := os.ReadFile(filepath.Join(src, rel)) //nolint:gosec // test temp dir
		_, _ = w.Write(b)
	}
	_ = zw.Close()
	body := buf.Bytes()
	sum := sha1.Sum(body) //nolint:gosec // matches the platform's hash
	url := "https://edge.forgecdn.net/files/1/2/skyblock.zip"
	f := modplatform.File{Name: "skyblock.zip", URL: url, Size: int64(len(body)), SHA1: hex.EncodeToString(sum[:])}

	eng, err := tasks.New(tasks.Config{Downloader: download.New(cdn{url: body}, ver), Root: root, HistoryPath: filepath.Join(root, "downloads.json"), Parallel: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	e.svc.d.Tasks, e.svc.d.Downloads = eng, filepath.Join(root, "cache", "worlds")

	for _, bad := range []modplatform.File{{SHA1: f.SHA1, Size: f.Size}, {URL: "https://evil.example/x.zip", SHA1: f.SHA1, Size: f.Size}, {URL: url, Size: f.Size}} {
		if _, err := e.svc.Install(e.id, "Skyblock", bad); err == nil {
			t.Fatalf("%+v accepted", bad)
		}
	}
	if _, err := e.svc.Install(e.id, "Skyblock", modplatform.File{SHA1: f.SHA1, Size: f.Size}); !errors.Is(err, ErrWebsiteOnly) {
		t.Fatalf("blocked file: %v", err)
	}
	job, err := e.svc.Install(e.id, "Skyblock", f)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Wait(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.insts.GameDir(e.id), "saves", "Skyblock", "level.dat")); err != nil {
		t.Fatalf("world not added: %v", err)
	}
	if left, _ := os.ReadDir(e.svc.d.Downloads); len(left) != 0 {
		t.Fatalf("download left behind: %v", left)
	}
}
