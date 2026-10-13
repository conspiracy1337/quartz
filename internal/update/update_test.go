package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/core/download"
	"quartz/internal/core/minisign"
	"quartz/internal/core/tasks"
)

const base = "https://github.example/owner/quartz/releases"

// cdn serves fixed bodies to both the manifest fetch and the download job.
type cdn map[string][]byte

func (c cdn) Get(_ context.Context, u string, _ http.Header) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	return c.Do(req)
}

func (c cdn) Do(req *http.Request) (*http.Response, error) {
	body, ok := c[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: 404, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
	return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Header: http.Header{}, Request: req}, nil
}

type env struct {
	u   *Updater
	cdn cdn
	sec minisign.SecretKey
	exe string
	dir string
}

func newEnv(t *testing.T, current string) *env {
	t.Helper()
	root := t.TempDir()
	pub, sec, err := minisign.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "bin", "quartz")
	_ = os.MkdirAll(filepath.Dir(exe), 0o700)
	if err := os.WriteFile(exe, []byte("old program"), 0o755); err != nil { //nolint:gosec // a test program
		t.Fatal(err)
	}
	c := cdn{}
	ver, err := download.OpenVerified(filepath.Join(root, "verified.json"), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := tasks.New(tasks.Config{Downloader: download.New(c, ver), Root: root, HistoryPath: filepath.Join(root, "downloads.json"), Parallel: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	dir := filepath.Join(root, "data", "update")
	u := New(Config{Repo: "owner/quartz", PublicKey: pub.String(), Version: current, Dir: dir, Exe: exe, HTTP: c, Tasks: eng,
		BaseURL: base, Platform: "linux-amd64"})
	return &env{u: u, cdn: c, sec: sec, exe: exe, dir: dir}
}

// release publishes version with a build whose content is body.
func (e *env) release(t *testing.T, version, body string, sign func([]byte) []byte) {
	t.Helper()
	sum := sha256.Sum256([]byte(body))
	m := Manifest{Version: version, Notes: "## New\n- Faster **starts**\n- Fixes", Files: map[string]File{
		"linux-amd64": {Name: "quartz-linux-amd64", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}}}
	b, _ := json.Marshal(m)
	if sign == nil {
		sign = func(b []byte) []byte { return e.sec.Sign(b, "quartz "+version) }
	}
	e.cdn[base+"/latest/download/update.json"] = b
	e.cdn[base+"/latest/download/update.json.minisig"] = sign(b)
	e.cdn[base+"/download/v"+version+"/quartz-linux-amd64"] = []byte(body)
}

func (e *env) waitReady(t *testing.T) State {
	t.Helper()
	for range 400 {
		if s := e.u.State(); s.Status == Ready || s.Status == Failed {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("never ready")
	return State{}
}

func TestUpdateEndToEnd(t *testing.T) {
	e := newEnv(t, "1.0.0")
	e.release(t, "1.1.0", "new program", nil)
	s, err := e.u.Check(context.Background())
	if err != nil || s.Status != Downloading || s.Version != "1.1.0" || strings.Join(s.Notes, "|") != "New|Faster starts|Fixes" {
		t.Fatalf("%+v %v", s, err)
	}
	if s = e.waitReady(t); s.Status != Ready {
		t.Fatalf("%+v", s)
	}
	if err := e.u.Install(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(e.exe); string(b) != "new program" {
		t.Fatal("not swapped")
	}
	if b, _ := os.ReadFile(e.exe + ".old"); string(b) != "old program" {
		t.Fatal("the old program must be kept until the new one starts")
	}
	if info, _ := os.Stat(e.exe); info.Mode()&0o100 == 0 {
		t.Fatal("the new program must be executable")
	}
	// The new version starts: success, and the old program goes.
	out := Finish(e.dir, "1.1.0")
	if !out.Updated || out.Version != "1.1.0" {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(e.exe + ".old"); !os.IsNotExist(err) {
		t.Fatal(".old kept")
	}
	if out := Finish(e.dir, "1.1.0"); out.Updated || out.Failed {
		t.Fatal("markers are read once")
	}
}

func TestUpdateRefusesBadReleases(t *testing.T) {
	e := newEnv(t, "1.0.0")
	_, other, _ := minisign.GenerateKey()
	e.release(t, "1.1.0", "evil", func(b []byte) []byte { return other.Sign(b, "x") })
	if _, err := e.u.Check(context.Background()); err == nil {
		t.Fatal("a release signed by another key must fail")
	}
	e.release(t, "0.9.0", "older", nil)
	if s, err := e.u.Check(context.Background()); err != nil || s.Status != Idle {
		t.Fatalf("an older version is no update: %+v %v", s, err)
	}
	// A build whose hash doesn't match fails the download.
	e.release(t, "1.2.0", "real", nil)
	e.cdn[base+"/download/v1.2.0/quartz-linux-amd64"] = []byte("fake")
	if _, err := e.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := e.waitReady(t); s.Status != Failed {
		t.Fatalf("%+v", s)
	}
	if err := e.u.Install(); !errors.Is(err, ErrNotReady) {
		t.Fatal(err)
	}
}

func TestOffForDevBuilds(t *testing.T) {
	e := newEnv(t, "0.0.0-dev")
	if _, err := e.u.Check(context.Background()); !errors.Is(err, ErrOff) || e.u.State().Status != Off {
		t.Fatal("development builds never update")
	}
}

func TestInterruptedSwapRestoresTheOldProgram(t *testing.T) {
	e := newEnv(t, "1.0.0")
	if err := os.Rename(e.exe, e.exe+".old"); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(filepath.Join(e.dir, lockFile), marker{From: "1.0.0", To: "1.1.0", Target: e.exe}); err != nil {
		t.Fatal(err)
	}
	out := Finish(e.dir, "1.0.0")
	if !out.Failed {
		t.Fatalf("%+v", out)
	}
	if b, _ := os.ReadFile(e.exe); string(b) != "old program" {
		t.Fatal("the old program must be put back")
	}
}

func TestTargetIsTheAppBundle(t *testing.T) {
	if got := Target(filepath.FromSlash("/Applications/Quartz.app/Contents/MacOS/quartz")); got != filepath.FromSlash("/Applications/Quartz.app") {
		t.Fatal(got)
	}
	if got := Target("/opt/quartz"); got != "/opt/quartz" {
		t.Fatal(got)
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.0.10", "1.0.9", true}, {"1.0.0", "1.0.0", false}, {"0.9.9", "1.0.0", false}, {"2.0.0", "0.0.0-dev", false}, {"x", "1.0.0", false}} {
		if newer(c.a, c.b) != c.want {
			t.Errorf("%s > %s", c.a, c.b)
		}
	}
}
