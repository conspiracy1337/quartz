// Package update keeps the launcher up to date from its GitHub releases. A
// release carries update.json (the version, its notes, and each platform's
// file with size and SHA-256) and update.json.minisig. The signature is
// checked against the public key built into the launcher before the
// manifest is even parsed, only newer versions are taken, and the download
// must match its size and hash. The new program replaces the old one with
// renames next to it (the old one kept as .old until the new one has
// started), and marker files tell the next start whether that worked.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/archive"
	"quartz/internal/core/download"
	"quartz/internal/core/events"
	"quartz/internal/core/hashx"
	"quartz/internal/core/minisign"
	"quartz/internal/core/tasks"
)

// Status is where the updater is.
type Status string

const (
	Off         Status = "off" // this build doesn't update itself
	Idle        Status = "idle"
	Checking    Status = "checking"
	Downloading Status = "downloading"
	Ready       Status = "ready" // downloaded and verified; installs on restart
	Failed      Status = "error"
)

// State is what the Updates tab and the banner show.
type State struct {
	Status  Status   `json:"status"`
	Current string   `json:"current"`
	Version string   `json:"version,omitempty"` // the newest version found
	Date    string   `json:"date,omitempty"`
	Notes   []string `json:"notes"` // release notes, one line each
	Error   string   `json:"error,omitempty"`
	Checked int64    `json:"checked,omitempty"` // Unix ms of the last check
	Job     string   `json:"job,omitempty"`
}

// Manifest is update.json.
type Manifest struct {
	Version string          `json:"version"`
	Date    string          `json:"date"`
	Notes   string          `json:"notes"`
	Files   map[string]File `json:"files"` // by platform: "linux-amd64", "windows-arm64", "darwin-universal"
}

// File is one platform's download.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Getter fetches a URL (netx.Client).
type Getter interface {
	Get(ctx context.Context, rawURL string, h http.Header) (*http.Response, error)
}

// Config wires the updater.
type Config struct {
	Repo      string // "owner/name" on GitHub
	PublicKey string // the minisign key releases are signed with
	Version   string // this build
	Dir       string // <data>/update
	Exe       string // the running program (resolved)
	HTTP      Getter
	Tasks     *tasks.Engine
	Bus       *events.Bus
	Log       *slog.Logger
	// BaseURL replaces https://github.com/<Repo>/releases (tests).
	BaseURL string
	// Platform replaces the running platform (tests).
	Platform string
}

// Updater is safe for concurrent use.
type Updater struct {
	cfg     Config
	key     minisign.PublicKey
	enabled bool

	mu     sync.Mutex
	state  State
	staged string // the verified new program (or .app) waiting to be installed
	busy   bool
}

var (
	// ErrOff means this build doesn't update itself.
	ErrOff = errors.New("update: updates are off in this build")
	// ErrNoBuild means the release has nothing for this platform.
	ErrNoBuild = errors.New("update: the newest release has no build for this computer")
	// ErrNotReady means there is no downloaded update to install.
	ErrNotReady = errors.New("update: no update is ready")
)

const (
	maxManifest = 1 << 20
	maxSig      = 4 << 10
	maxBuild    = 1 << 30
)

var (
	versionRe = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}$`)
	nameRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	sha256Re  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// New returns an updater. It is off for development builds, builds
// without a public key, and unknown platforms.
func New(cfg Config) *Updater {
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://github.com/" + cfg.Repo + "/releases"
	}
	if cfg.Platform == "" {
		cfg.Platform = Platform()
	}
	u := &Updater{cfg: cfg, state: State{Status: Off, Current: cfg.Version, Notes: []string{}}}
	key, err := minisign.ParsePublicKey(cfg.PublicKey)
	if err == nil && versionRe.MatchString(cfg.Version) && cfg.Repo != "" && cfg.Exe != "" {
		u.key, u.enabled = key, true
		u.state.Status = Idle
	}
	return u
}

// Platform names this computer's build in the manifest.
func Platform() string {
	if runtime.GOOS == "darwin" {
		return "darwin-universal"
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

// State returns the current state.
func (u *Updater) State() State {
	u.mu.Lock()
	defer u.mu.Unlock()
	s := u.state
	s.Notes = append([]string{}, s.Notes...)
	return s
}

func (u *Updater) set(fn func(*State)) State {
	u.mu.Lock()
	fn(&u.state)
	s := u.state
	u.mu.Unlock()
	if u.cfg.Bus != nil {
		u.cfg.Bus.Emit(events.UpdateState, s)
	}
	return s
}

// Check asks GitHub for the newest release and, when it is newer, downloads
// it in the background as a job. It returns the state once the check is
// done (the download carries on).
func (u *Updater) Check(ctx context.Context) (State, error) {
	if !u.enabled {
		return u.State(), ErrOff
	}
	u.mu.Lock()
	if u.busy || u.state.Status == Ready {
		s := u.state
		u.mu.Unlock()
		return s, nil
	}
	u.busy = true
	u.mu.Unlock()
	release := func() { u.mu.Lock(); u.busy = false; u.mu.Unlock() }

	u.set(func(s *State) { s.Status, s.Error = Checking, "" })
	m, err := u.manifest(ctx)
	now := time.Now().UnixMilli()
	if err != nil {
		release()
		return u.set(func(s *State) { s.Status, s.Error, s.Checked = Failed, err.Error(), now }), err
	}
	if !newer(m.Version, u.cfg.Version) {
		release()
		return u.set(func(s *State) { s.Status, s.Checked, s.Version = Idle, now, "" }), nil
	}
	f, ok := m.Files[u.cfg.Platform]
	if !ok || !nameRe.MatchString(f.Name) || !sha256Re.MatchString(f.SHA256) || f.Size <= 0 || f.Size > maxBuild {
		release()
		return u.set(func(s *State) { s.Status, s.Error, s.Checked = Failed, ErrNoBuild.Error(), now }), ErrNoBuild
	}
	job, err := u.download(m, f)
	if err != nil {
		release()
		return u.set(func(s *State) { s.Status, s.Error, s.Checked = Failed, err.Error(), now }), err
	}
	return u.set(func(s *State) {
		s.Status, s.Checked, s.Version, s.Date, s.Notes, s.Job = Downloading, now, m.Version, m.Date, noteLines(m.Notes), job
	}), nil
}

// manifest fetches update.json and its signature and verifies it before
// reading it.
func (u *Updater) manifest(ctx context.Context) (Manifest, error) {
	base := u.cfg.BaseURL + "/latest/download/"
	body, err := u.fetch(ctx, base+"update.json", maxManifest)
	if err != nil {
		return Manifest{}, err
	}
	sig, err := u.fetch(ctx, base+"update.json.minisig", maxSig)
	if err != nil {
		return Manifest{}, err
	}
	if _, err := u.key.Verify(body, sig); err != nil {
		return Manifest{}, fmt.Errorf("update: the release isn't signed with this launcher's key: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil || !versionRe.MatchString(m.Version) {
		return Manifest{}, errors.New("update: the release manifest is malformed")
	}
	return m, nil
}

func (u *Updater) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := u.cfg.HTTP.Get(ctx, url, http.Header{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: %s answered %s", strings.TrimPrefix(url, "https://"), resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("update: the release file is too large")
	}
	return b, nil
}

// download fetches the build as a job and stages it when it completes.
func (u *Updater) download(m Manifest, f File) (string, error) {
	if err := os.MkdirAll(u.cfg.Dir, 0o700); err != nil {
		return "", err
	}
	dest := filepath.Join(u.cfg.Dir, f.Name)
	it := download.Item{URLs: []string{u.cfg.BaseURL + "/download/v" + m.Version + "/" + f.Name}, Dest: dest, Size: f.Size, Sum: hashx.Of(hashx.SHA256, f.SHA256)}
	spec := tasks.NewJob(tasks.Meta{Title: "Quartz Client " + m.Version, Subtitle: "Launcher update", Kind: "update",
		Art: tasks.Art{Background: "#0f0f11", Initial: "Q"}}).Step("Launcher", tasks.Bytes, tasks.Download(it))
	job, err := u.cfg.Tasks.Submit(spec)
	if err != nil {
		return "", err
	}
	go func() { //nolint:gosec // G118: the download outlives the check
		defer func() { u.mu.Lock(); u.busy = false; u.mu.Unlock() }()
		if err := u.cfg.Tasks.Wait(context.Background(), job); err != nil {
			u.set(func(s *State) { s.Status, s.Error = Failed, err.Error() })
			return
		}
		staged, err := u.stage(dest)
		if err != nil {
			u.cfg.Log.Warn("staging the update failed", "err", err)
			u.set(func(s *State) { s.Status, s.Error = Failed, err.Error() })
			return
		}
		u.mu.Lock()
		u.staged = staged
		u.mu.Unlock()
		u.set(func(s *State) { s.Status, s.Job = Ready, "" })
	}()
	return job, nil
}

// stage prepares a verified download for the swap: the program itself, or
// on macOS the .app unpacked from its zip.
func (u *Updater) stage(file string) (string, error) {
	if !strings.HasSuffix(u.cfg.Platform, "universal") {
		return file, nil
	}
	out := filepath.Join(u.cfg.Dir, "app")
	_ = os.RemoveAll(out)
	if err := archive.ExtractZip(file, out, archive.Options{MaxTotalSize: 2 * maxBuild}); err != nil {
		return "", err
	}
	apps, _ := filepath.Glob(filepath.Join(out, "*.app"))
	if len(apps) != 1 {
		return "", errors.New("update: the macOS download holds no app")
	}
	return apps[0], nil
}

// newer reports whether version a is newer than b (both x.y.z).
func newer(a, b string) bool {
	if !versionRe.MatchString(a) || !versionRe.MatchString(b) {
		return false
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range 3 {
		var x, y int
		_, _ = fmt.Sscan(pa[i], &x)
		_, _ = fmt.Sscan(pb[i], &y)
		if x != y {
			return x > y
		}
	}
	return false
}

// noteLines turns Markdown release notes into plain lines: list markers,
// headings and emphasis are dropped, blank lines skipped.
func noteLines(md string) []string {
	out := []string{}
	for _, l := range strings.Split(md, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimLeft(l, "#")
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(l, "- "), "* "))
		l = strings.NewReplacer("**", "", "__", "", "`", "").Replace(l)
		if l != "" && len(out) < 100 {
			out = append(out, l)
		}
	}
	return out
}
