// Package sandbox runs the game (and a hosted server) so that mods reach
// only what the instance's rules allow: bubblewrap with a seccomp filter on
// Linux, Seatbelt on macOS and an AppContainer on Windows. It fails closed:
// a sandboxed run whose sandbox can't be built doesn't start.
package sandbox

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"quartz/internal/core/fsx"
	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

// Status says whether this system can sandbox games, for the Sandbox page.
type Status struct {
	Available bool   `json:"available"`
	Name      string `json:"name"`    // "bubblewrap", "Seatbelt", "AppContainer"
	Version   string `json:"version"` // "0.11.1"
	Reason    string `json:"reason"`  // why not, when unavailable
}

// Run is what one sandboxed run may reach, as the launcher knows it. Every
// path is an absolute host path.
type Run struct {
	Instance string
	Rules    rules.Settings
	GameDir  string // where the game runs: read-write
	Scratch  string // this run's own folder (exists; the sandbox may write files there)
	Data     string // the instance's persistent sandbox data (caches, mods' own config)
	Read     []string
	// Write are other read-write folders (a hosted server's worlds).
	Write []string
	// Redirects are folders inside read-only ones the game must write (its
	// skin cache under the shared assets).
	Redirects []Redirect
	// Shared are the shared folders the run's view uses read-write; on
	// Windows they need the container's access too.
	Shared []string
	// Mounts are the run view's bubblewrap mounts (Linux).
	Mounts []string
	// Headless is a server: no display, audio or Discord.
	Headless bool
	// GameMode registers the game with GameMode from outside (Linux).
	GameMode bool
}

// protectedFolders are the game folders a run reads but never changes. A
// mod's file in mods/ is its store blob, a hard link (where the disk can't
// clone) in every instance that uses it, so a game able to write one would
// change it for all of them, unsandboxed ones included.
var protectedFolders = []string{"mods"}

// Protected lists the run's protected folders that exist as real folders (a
// mods folder linked to another launcher's holds no store files).
func (r Run) Protected() []string {
	var out []string
	for _, f := range protectedFolders {
		p := filepath.Join(r.GameDir, f)
		if fi, err := os.Lstat(p); err == nil && fi.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// Redirect shows the instance's own From folder at To, read-write, where
// the OS can mount it (Linux). Elsewhere To itself becomes writable.
type Redirect struct{ From, To string }

// Level is the run's level for a rule on this OS.
func (r Run) Level(id rules.ID) rules.Level { return r.Rules.Level(id, runtime.GOOS) }

// Jail is one run's sandbox. Close it after the process exits.
type Jail struct {
	Name    string // the backend, for the run record
	started func(pid int)
	closers []func()
}

// Started tells the jail the process is running.
func (j *Jail) Started(pid int) {
	if j != nil && j.started != nil {
		j.started(pid)
	}
}

// Close releases what the jail held.
func (j *Jail) Close() {
	if j == nil {
		return
	}
	for i := len(j.closers) - 1; i >= 0; i-- {
		j.closers[i]()
	}
	j.closers = nil
}

// Config wires a Service.
type Config struct {
	DataRoot string // the launcher's data folder: never reachable as a whole
	Log      *slog.Logger
}

// Service builds jails.
type Service struct {
	c    Config
	home string
	b    backend
}

// backend is one OS's sandbox.
type backend interface {
	status() Status
	// wrap turns spec into the sandboxed process and returns its jail.
	wrap(s *Service, r Run, spec procx.Spec) (procx.Spec, *Jail, error)
}

// New returns a Service for this OS.
func New(c Config) *Service {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	home, _ := os.UserHomeDir()
	return &Service{c: c, home: home, b: newBackend()}
}

// Status probes (once) whether games can be sandboxed here.
func (s *Service) Status() Status { return s.b.status() }

// ErrUnavailable is returned when the sandbox can't start on this system.
var ErrUnavailable = errors.New("the sandbox can't start")

// Wrap returns spec run inside r's sandbox. It fails closed: nothing runs
// unsandboxed because the sandbox couldn't be built.
func (s *Service) Wrap(r Run, spec procx.Spec) (procx.Spec, *Jail, error) {
	st := s.Status()
	if !st.Available {
		return procx.Spec{}, nil, fmt.Errorf("%w: %s", ErrUnavailable, st.Reason)
	}
	for _, f := range r.Rules.Folders {
		if err := s.CheckFolder(f.Path); err != nil {
			return procx.Spec{}, nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
	}
	dirs := append([]string{r.Scratch, r.GameDir}, r.Write...)
	for _, d := range r.Redirects {
		dirs = append(dirs, d.From, d.To)
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return procx.Spec{}, nil, err
		}
	}
	if r.Data != "" {
		for _, d := range []string{"cache", "config", "data"} {
			if err := os.MkdirAll(filepath.Join(r.Data, d), 0o700); err != nil {
				return procx.Spec{}, nil, err
			}
		}
	}
	out, j, err := s.b.wrap(s, r, spec)
	if err != nil {
		return procx.Spec{}, nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	s.c.Log.Info("sandbox", "instance", r.Instance, "backend", j.Name, "preset", r.Rules.Preset)
	return out, j, nil
}

// ErrFolder is why a folder can't be allowed.
var ErrFolder = errors.New("that folder can't be allowed")

// systemRoots can never be allowed: the system itself.
// CheckFolder refuses a folder the player may not allow: missing, not a
// folder, the system, or one that is or holds the launcher's data or the
// home folder (the home folder has its own rule).
func (s *Service) CheckFolder(p string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%w: it isn't a full path", ErrFolder)
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return fmt.Errorf("%w: it doesn't exist", ErrFolder)
	}
	if fi, err := os.Stat(real); err != nil || !fi.IsDir() {
		return fmt.Errorf("%w: it isn't a folder", ErrFolder)
	}
	for _, q := range []string{p, real} {
		clean := filepath.Clean(q)
		if fsx.IsSystemRoot(clean) {
			return fmt.Errorf("%w: it's part of the system", ErrFolder)
		}
		if s.c.DataRoot != "" && (fsx.Within(clean, s.c.DataRoot) || fsx.Within(s.c.DataRoot, clean)) {
			return fmt.Errorf("%w: it holds Quartz's data", ErrFolder)
		}
		if s.home != "" && fsx.Within(s.home, clean) {
			return fmt.Errorf("%w: it holds your home folder (use the home folder row)", ErrFolder)
		}
	}
	return nil
}

// Forget drops what an instance's sandbox keeps between runs (its Windows
// app container); call it when the instance is deleted for good.
func (s *Service) Forget(instance string) error { return forget(instance) }

// JavaRoot is the runtime folder holding a java binary (…/bin/java), or a
// macOS bundle's Contents folder, so its libraries come along.
func JavaRoot(bin string) string {
	root := filepath.Dir(filepath.Dir(bin))
	if filepath.Base(root) == "Home" && filepath.Base(filepath.Dir(root)) == "Contents" {
		root = filepath.Dir(root)
	}
	return root
}
