// Package java finds, probes, downloads and picks Java runtimes. Detection
// covers every place Prism looks; each candidate is probed once through the
// wrapper jar (results cached by size and modification time); missing
// versions are installed from Mojang, Eclipse Temurin or Azul Zulu as part of
// a download job.
package java

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"quartz/internal/core/fsx"
	"quartz/internal/core/httpcache"
	"quartz/internal/core/jsonfile"
	"quartz/internal/core/procx"
	"quartz/internal/meta"
	"quartz/internal/wrapperjar"
)

// Source says where a runtime came from (the badge in Settings → Java).
type Source string

const (
	Managed Source = "Managed" // installed by the launcher
	System  Source = "System"
	SDKMAN  Source = "SDKMAN"
	Found   Source = "Found"
)

// Runtime is one usable Java installation.
type Runtime struct {
	Path    string `json:"path"` // the java binary (console variant on Windows)
	Major   int    `json:"major"`
	Version string `json:"version"`
	Vendor  string `json:"vendor"`
	Arch    string `json:"arch"` // x86_64 | arm64 | x86
	Source  Source `json:"source"`
}

// Name is the label shown in the UI, e.g. "Eclipse Temurin 21.0.5".
func (r Runtime) Name() string {
	v := r.Vendor
	if v == "" {
		v = "Java"
	}
	return v + " " + r.Version
}

// Errors callers branch on.
var (
	ErrNoRuntime = errors.New("java: no runtime is available for this platform")
	ErrNotJava   = errors.New("java: that is not a working Java installation")
)

const probeTimeout = 15 * time.Second

// Config wires a Manager.
type Config struct {
	Dir       string // managed runtimes (store/java)
	StatePath string // probe cache and user-added runtimes (java.json)
	BinDir    string // where the wrapper jar is installed (store/bin)
	Meta      *meta.Service
	Cache     *httpcache.Cache
	Vendor    func() string // the "download missing versions from" setting
	Log       *slog.Logger
}

type probeRecord struct {
	Size    int64   `json:"size"`
	ModTime int64   `json:"mtime"`
	Runtime Runtime `json:"runtime"`
}

type stateDoc struct {
	Probes map[string]probeRecord `json:"probes"`
	Extra  []string               `json:"extra"` // java binaries the user added
}

// Manager is safe for concurrent use.
type Manager struct {
	cfg   Config
	state *jsonfile.Store[stateDoc]
	env   searchEnv
	goos  string
	arch  string

	mu       sync.Mutex
	detected []Runtime
	scanned  bool
}

// New returns a Manager.
func New(c Config) (*Manager, error) {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	st, err := jsonfile.Open(jsonfile.Options[stateDoc]{
		Path:    c.StatePath,
		Default: func() stateDoc { return stateDoc{Probes: map[string]probeRecord{}} },
		Normalize: func(d stateDoc) stateDoc {
			if d.Probes == nil {
				d.Probes = map[string]probeRecord{}
			}
			return d
		},
		Log: c.Log,
	})
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	return &Manager{
		cfg: c, state: st, goos: runtime.GOOS, arch: hostArch(runtime.GOARCH),
		env: searchEnv{goos: runtime.GOOS, home: home, getenv: os.Getenv},
	}, nil
}

// Flush saves the probe cache.
func (m *Manager) Flush() error { return m.state.Flush() }

func hostArch(goarch string) string {
	switch goarch {
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return "x86_64"
	}
}

// Runtimes lists managed and detected runtimes, newest major first. The
// first call scans; later calls reuse that scan until Scan is called.
func (m *Manager) Runtimes(ctx context.Context) []Runtime {
	m.mu.Lock()
	scanned := m.scanned
	m.mu.Unlock()
	if !scanned {
		m.Scan(ctx)
	}
	m.mu.Lock()
	out := append(m.managed(), m.detected...)
	m.mu.Unlock()
	slices.SortStableFunc(out, func(a, b Runtime) int { return b.Major - a.Major })
	return out
}

// Scan looks for Java again and probes anything new or changed.
func (m *Manager) Scan(ctx context.Context) {
	found := map[string]Source{}
	var order []string
	add := func(path string, src Source) {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return
		}
		if info, err := os.Stat(real); err != nil || !info.Mode().IsRegular() {
			return
		}
		if _, ok := found[real]; !ok {
			found[real] = src
			order = append(order, real)
		}
	}
	for _, c := range candidates(m.env) {
		add(c.path, c.source)
	}
	for _, p := range m.state.Get().Extra {
		add(p, Found)
	}
	managedDir, _ := filepath.EvalSymlinks(m.cfg.Dir)

	results := make([]*Runtime, len(order))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for i, p := range order {
		if managedDir != "" && strings.HasPrefix(p, managedDir+string(filepath.Separator)) {
			continue // listed from their markers instead
		}
		g.Go(func() error {
			if rt, err := m.probeCached(gctx, p); err == nil {
				rt.Source = found[p]
				results[i] = &rt
			}
			return nil
		})
	}
	_ = g.Wait()
	var list []Runtime
	for _, r := range results {
		if r != nil {
			list = append(list, *r)
		}
	}
	m.mu.Lock()
	m.detected, m.scanned = list, true
	m.mu.Unlock()
}

// probeCached probes path unless an unchanged binary was probed before.
func (m *Manager) probeCached(ctx context.Context, path string) (Runtime, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Runtime{}, err
	}
	if rec, ok := m.state.Get().Probes[path]; ok && rec.Size == info.Size() && rec.ModTime == info.ModTime().UnixNano() {
		return rec.Runtime, nil
	}
	rt, err := m.Probe(ctx, path)
	if err != nil {
		return Runtime{}, err
	}
	_, _ = m.state.Update(func(d *stateDoc) error {
		d.Probes[path] = probeRecord{Size: info.Size(), ModTime: info.ModTime().UnixNano(), Runtime: rt}
		return nil
	})
	return rt, nil
}

var probeKeys = []string{"java.specification.version", "java.version", "java.vendor", "os.arch"}

// Probe runs the binary through the wrapper jar and reads its properties.
func (m *Manager) Probe(ctx context.Context, javaPath string) (Runtime, error) {
	jar, err := wrapperjar.Install(m.cfg.BinDir)
	if err != nil {
		return Runtime{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	script := new(wrapperjar.Script)
	for _, k := range probeKeys {
		script.Print(k)
	}
	cmd := exec.CommandContext(ctx, javaPath, "-Xshare:auto", "-cp", jar, wrapperjar.MainClass) //nolint:gosec // the binary is a detected or user-chosen java; no shell is involved
	cmd.Env = CleanEnv(os.Environ())
	cmd.Stdin = bytes.NewReader(script.Exit().Bytes())
	var out capped
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	procx.NoConsole(cmd)
	if err := cmd.Run(); err != nil {
		return Runtime{}, fmt.Errorf("%w: %s: %w", ErrNotJava, javaPath, err)
	}
	return parseProbe(javaPath, out.String())
}

func parseProbe(javaPath, out string) (Runtime, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < len(probeKeys) {
		return Runtime{}, fmt.Errorf("%w: %s", ErrNotJava, javaPath)
	}
	vals := make([]string, len(probeKeys))
	for i := range probeKeys {
		v, _, err := wrapperjar.ParseValue(lines[i])
		if err != nil {
			return Runtime{}, fmt.Errorf("%w: %s", ErrNotJava, javaPath)
		}
		vals[i] = v
	}
	major, err := majorOf(vals[0])
	if err != nil {
		return Runtime{}, fmt.Errorf("%w: %s", ErrNotJava, javaPath)
	}
	return Runtime{Path: javaPath, Major: major, Version: vals[1], Vendor: vals[2], Arch: normalizeArch(vals[3])}, nil
}

// majorOf turns "1.8" or "21" into 8 or 21.
func majorOf(spec string) (int, error) {
	spec = strings.TrimPrefix(spec, "1.")
	n, err := strconv.Atoi(strings.SplitN(spec, ".", 2)[0])
	if err != nil || n < 5 || n > 99 {
		return 0, ErrNotJava
	}
	return n, nil
}

func normalizeArch(a string) string {
	switch strings.ToLower(a) {
	case "amd64", "x86_64":
		return "x86_64"
	case "aarch64", "arm64":
		return "arm64"
	case "x86", "i386", "i486", "i586", "i686":
		return "x86"
	default:
		return a
	}
}

// capped keeps at most 64 KB of probe output.
type capped struct{ bytes.Buffer }

func (c *capped) Write(p []byte) (int, error) {
	if room := 64<<10 - c.Len(); room > 0 {
		c.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// AddFolder registers a Java installation the user picked; dir may be the
// installation folder or its bin folder.
func (m *Manager) AddFolder(ctx context.Context, dir string) (Runtime, error) {
	bins := findBinaries(dir, exe(m.goos), 4)
	if direct := filepath.Join(dir, exe(m.goos)); fileExists(direct) {
		bins = append([]string{direct}, bins...)
	}
	for _, b := range bins {
		rt, err := m.probeCached(ctx, b)
		if err != nil {
			continue
		}
		rt.Source = Found
		_, _ = m.state.Update(func(d *stateDoc) error {
			if !slices.Contains(d.Extra, b) {
				d.Extra = append(d.Extra, b)
			}
			return nil
		})
		m.mu.Lock()
		if !slices.ContainsFunc(m.detected, func(r Runtime) bool { return r.Path == b }) {
			m.detected = append(m.detected, rt)
		}
		m.mu.Unlock()
		return rt, nil
	}
	return Runtime{}, ErrNotJava
}

// Request describes the Java a launch needs.
type Request struct {
	Major     int
	Component string // Mojang's component name, when known
	Override  string // instance setting: a specific java binary, or ""
}

// Installed finds a runtime for req once it is installed (after a job's Java
// step), for tools that run during an install such as a loader's processors.
func (m *Manager) Installed(req Request) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		rt, plan, err := m.Resolve(ctx, req)
		if err != nil {
			return "", err
		}
		if plan != nil || rt.Path == "" {
			return "", fmt.Errorf("java: Java %d is needed to finish installing the mod loader", req.Major)
		}
		return rt.Path, nil
	}
}

// Resolve picks the runtime for req: the instance override, then a managed
// runtime of exactly that major, then a detected one of the host's
// architecture. When none fits it returns an install plan instead.
func (m *Manager) Resolve(ctx context.Context, req Request) (Runtime, *Plan, error) {
	if req.Override != "" {
		rt, err := m.probeCached(ctx, req.Override)
		return rt, nil, err
	}
	for _, rt := range m.Runtimes(ctx) {
		if rt.Major == req.Major && (rt.Arch == m.arch || rt.Arch == "") {
			return rt, nil, nil
		}
	}
	p, err := m.Plan(ctx, req.Major, req.Component)
	return Runtime{}, p, err
}

// managed lists runtimes the launcher installed (from their markers).
func (m *Manager) managed() []Runtime {
	entries, err := os.ReadDir(m.cfg.Dir)
	if err != nil {
		return nil
	}
	var out []Runtime
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(m.cfg.Dir, e.Name())
		mk, err := readMarker(dir)
		if err != nil {
			continue
		}
		bin, err := fsx.SafeJoin(dir, mk.Java)
		if err != nil || !fileExists(bin) {
			continue
		}
		out = append(out, Runtime{Path: bin, Major: mk.Major, Version: mk.Version, Vendor: vendorNames[mk.Vendor], Arch: m.arch, Source: Managed})
	}
	return out
}

// marker is written last when a managed runtime is complete.
type marker struct {
	Vendor  string `json:"vendor"`
	Major   int    `json:"major"`
	Version string `json:"version"`
	Java    string `json:"java"` // slash path of the java binary inside the folder
}

const markerName = "runtime.json"

func readMarker(dir string) (marker, error) {
	var mk marker
	data, err := fsx.ReadCapped(filepath.Join(dir, markerName), 64<<10)
	if err != nil {
		return mk, err
	}
	if err := json.Unmarshal(data, &mk); err != nil || mk.Major <= 0 || !filepath.IsLocal(filepath.FromSlash(mk.Java)) {
		return mk, ErrNotJava
	}
	return mk, nil
}

func writeMarker(dir string, mk marker) error {
	data, err := json.Marshal(mk)
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(filepath.Join(dir, markerName), data, fsx.FilePerm)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}
