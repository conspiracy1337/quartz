// Package runview assembles what one run of the game sees: the instance's
// game folder with its shared folders in place, built from OS features
// instead of links (Mod Organizer 2's idea, without its DLL hooking).
//
// The first run to use a folder gets it directly: on Linux a bind mount in
// a bubblewrap mount namespace, on Windows a junction made for the run, and
// elsewhere a copy-on-write clone merged back on exit. A run that starts
// while another uses the same folder gets a private view instead (an
// overlay on Linux, a clone elsewhere), whose changes merge back on exit
// without losing the other run's (see merge.go).
package runview

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"quartz/internal/core/bwrap"
	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
)

// Folder is a shared folder a run assembles into the game folder.
type Folder struct {
	Rel    string // inside the game folder, slash-separated ("saves")
	Shared string // the one real copy
	Mode   Mode
}

// Mode is how a run sees a shared folder.
type Mode string

// Modes. Private and Empty are the sandbox's "Read only" and "Blocked":
// the run may write, and nothing it writes is kept.
const (
	Shared  Mode = ""        // the real shared copy, changes kept
	Private Mode = "private" // a throwaway layer over the shared copy
	Empty   Mode = "empty"   // an empty throwaway folder
)

// Request describes one run.
type Request struct {
	Game    string // the instance's game folder
	Scratch string // this run's own folder (created, removed on Close)
	Folders []Folder
}

// Config wires an Assembler.
type Config struct {
	// Retitle renames a world kept as "W (copy 2)".
	Retitle func(dir, suffix string) error
	Log     *slog.Logger
	// Backend forces "bwrap" or "clone" (tests); "" picks the best one.
	Backend string
}

// Assembler builds views and tracks which folders live runs use.
type Assembler struct {
	c Config

	mu     sync.Mutex
	leases map[string]int
}

// New returns an Assembler.
func New(c Config) *Assembler {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	return &Assembler{c: c, leases: map[string]int{}}
}

// lease marks folders used by one more run and reports, per folder,
// whether this run is the only one (it may then use the folder directly).
func (a *Assembler) lease(paths []string) map[string]bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	only := map[string]bool{}
	for _, p := range paths {
		only[p] = a.leases[p] == 0
		a.leases[p]++
	}
	return only
}

func (a *Assembler) release(paths []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, p := range paths {
		if a.leases[p]--; a.leases[p] <= 0 {
			delete(a.leases, p)
		}
	}
}

// Mount kinds.
const (
	direct   = "direct"   // the real folder, nothing to merge
	bind     = "bind"     // bubblewrap bind mount of the real folder
	overlay  = "overlay"  // bubblewrap overlay, merged back from its upper folder
	clone    = "clone"    // a private copy, merged back from a snapshot diff
	junction = "junction" // a Windows junction to the real folder
	scratch  = "scratch"  // a throwaway folder, never merged back
)

// mount is one folder of the view and how its changes come back.
type mount struct {
	Kind   string   `json:"kind"`
	Path   string   `json:"path"`   // where the game sees it
	Target string   `json:"target"` // the real folder
	Worlds string   `json:"worlds"` // see merger.worlds
	Upper  string   `json:"upper,omitempty"`
	Work   string   `json:"work,omitempty"`
	Snap   string   `json:"snap,omitempty"` // clone: its snapshot file
	Skip   []string `json:"skip,omitempty"` // mount points of the other mounts
	// InPlace marks a clone inside the instance's own game folder, which is
	// emptied again after the run.
	InPlace bool `json:"inPlace,omitempty"`
}

// state is saved in the scratch folder, so a view the launcher couldn't
// close (it quit or crashed while the game ran) is merged back later.
type state struct {
	PID     int       `json:"pid"`
	Start   time.Time `json:"start"`
	Game    string    `json:"game"`
	GameDir string    `json:"gameDir"`
	Scratch string    `json:"scratch"`
	// Bin and Bwrap are bubblewrap and the view's mounts (no root, no command).
	Bin    string   `json:"bin,omitempty"`
	Bwrap  []string `json:"bwrap,omitempty"`
	Mounts []mount  `json:"mounts"`
	Leased []string `json:"leased"`
}

const stateFile = "view.json"

// View is one run's assembled folder.
type View struct {
	a  *Assembler
	st state
}

// GameDir is where the game runs (--gameDir and its working folder).
func (v *View) GameDir() string { return v.st.GameDir }

// Wrap prefixes the game's command with bubblewrap when the view needs a
// mount namespace; the rest of the system is shared as is.
func (v *View) Wrap(argv []string) []string {
	if v.st.Bin == "" {
		return argv
	}
	b := new(bwrap.Args).Add(v.st.Bin).DevBind("/", "/").Add(v.st.Bwrap...)
	return append(b.List(), argv...)
}

// Mounts are the view's bubblewrap mounts, for a sandbox that builds its own
// root: nil when the view needs no namespace.
func (v *View) Mounts() []string { return slices.Clone(v.st.Bwrap) }

// Backend names how the view was built, for the log.
func (v *View) Backend() string {
	if v.st.Bin != "" {
		return "bwrap"
	}
	for _, m := range v.st.Mounts {
		if m.Kind != direct {
			return m.Kind
		}
	}
	return direct
}

// Started records the game's process, so recovery can tell it has gone.
func (v *View) Started(pid int) {
	v.st.PID = pid
	v.save()
}

func (v *View) save() {
	if data, err := json.Marshal(v.st); err == nil {
		_ = fsx.AtomicWrite(filepath.Join(v.st.Scratch, stateFile), data, fsx.FilePerm)
	}
}

// Assemble builds the view for one run. Close it after the game exits.
func (a *Assembler) Assemble(r Request) (*View, error) {
	leased := []string{r.Game}
	for _, f := range r.Folders {
		if f.Mode == Shared {
			leased = append(leased, f.Shared)
		}
	}
	only := a.lease(leased)
	v := &View{a: a, st: state{Start: time.Now(), Game: r.Game, GameDir: r.Game, Scratch: r.Scratch, Leased: leased}}
	err := a.assemble(v, r, only)
	if err != nil {
		_, cerr := v.Close(context.Background())
		return nil, errors.Join(err, cerr)
	}
	v.save()
	return v, nil
}

func (a *Assembler) assemble(v *View, r Request, only map[string]bool) error {
	if err := os.MkdirAll(r.Scratch, 0o700); err != nil {
		return err
	}
	skip := make([]string, 0, len(r.Folders))
	for _, f := range r.Folders {
		skip = append(skip, f.Rel)
		if err := os.MkdirAll(filepath.Join(r.Game, filepath.FromSlash(f.Rel)), 0o700); err != nil {
			return err
		}
		if err := os.MkdirAll(f.Shared, 0o700); err != nil {
			return err
		}
	}
	gameWorlds := "saves"
	if slices.Contains(skip, "saves") {
		gameWorlds = "-"
	}
	private := !only[r.Game]
	overlays := private
	for _, f := range r.Folders {
		private = private || (f.Mode == Shared && !only[f.Shared])
		overlays = overlays || f.Mode == Private || (f.Mode == Shared && !only[f.Shared])
	}
	sup := bwrap.Detect()
	backend := a.c.Backend
	if backend == "" {
		backend = "clone"
		if sup.Bin != "" && (!overlays || sup.Overlay) {
			backend = "bwrap"
		}
	}
	if backend == "bwrap" && len(r.Folders) == 0 && !private {
		backend = "clone" // nothing to assemble: run in place, no namespace
	}
	if backend == "bwrap" {
		return a.assembleBwrap(v, r, only, skip, gameWorlds, sup.Bin)
	}
	return a.assembleClone(v, r, only, skip, gameWorlds)
}

// assembleBwrap: the game folder in place or as an overlay, shared folders
// bind-mounted (or overlaid) at their place inside it, so every path the
// game sees is the one the launcher knows.
func (a *Assembler) assembleBwrap(v *View, r Request, only map[string]bool, skip []string, gameWorlds, bin string) error {
	if bin == "" {
		bin = "bwrap"
	}
	var args bwrap.Args
	add := func(i int, lower, at string, worlds string, s []string) error {
		m := mount{Kind: overlay, Path: at, Target: lower, Worlds: worlds, Skip: s,
			Upper: filepath.Join(r.Scratch, "upper"+strconv.Itoa(i)), Work: filepath.Join(r.Scratch, "work"+strconv.Itoa(i))}
		for _, d := range []string{m.Upper, m.Work} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				return err
			}
		}
		args.Overlay(lower, m.Upper, m.Work, at)
		v.st.Mounts = append(v.st.Mounts, m)
		return nil
	}
	if only[r.Game] {
		v.st.Mounts = append(v.st.Mounts, mount{Kind: direct, Path: r.Game, Target: r.Game})
	} else if err := add(0, r.Game, r.Game, gameWorlds, skip); err != nil {
		return err
	}
	for i, f := range r.Folders {
		at := filepath.Join(r.Game, filepath.FromSlash(f.Rel))
		worlds := worldsOf(f.Rel)
		switch {
		case f.Mode == Private:
			args.TmpOverlay(f.Shared, at)
			v.st.Mounts = append(v.st.Mounts, mount{Kind: scratch, Path: at, Target: f.Shared})
		case f.Mode == Empty:
			args.Tmpfs(at)
			v.st.Mounts = append(v.st.Mounts, mount{Kind: scratch, Path: at})
		case only[f.Shared]:
			args.Bind(f.Shared, at)
			v.st.Mounts = append(v.st.Mounts, mount{Kind: bind, Path: at, Target: f.Shared, Worlds: worlds})
		default:
			if err := add(i+1, f.Shared, at, worlds, nil); err != nil {
				return err
			}
		}
	}
	v.st.Bin, v.st.Bwrap = bin, args.List()
	return nil
}

// assembleClone: the game folder in place, or a private clone of it; shared
// folders as a junction (Windows, first user) or a private clone.
func (a *Assembler) assembleClone(v *View, r Request, only map[string]bool, skip []string, gameWorlds string) error {
	if only[r.Game] {
		v.st.Mounts = append(v.st.Mounts, mount{Kind: direct, Path: r.Game, Target: r.Game})
	} else {
		at := filepath.Join(r.Scratch, "game")
		if err := linkx.CloneTree(r.Game, at); err != nil {
			return err
		}
		m := mount{Kind: clone, Path: at, Target: r.Game, Worlds: gameWorlds, Skip: skip}
		if err := a.snap(&m, r.Scratch, 0); err != nil {
			return err
		}
		v.st.Mounts = append(v.st.Mounts, m)
		v.st.GameDir = at
	}
	for i, f := range r.Folders {
		at := filepath.Join(v.st.GameDir, filepath.FromSlash(f.Rel))
		if err := os.Remove(at); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err // a mount point must be empty (syncing empties it before each launch)
		}
		inPlace := v.st.GameDir == r.Game
		switch f.Mode {
		case Empty:
			if err := os.MkdirAll(at, 0o700); err != nil {
				return err
			}
			v.st.Mounts = append(v.st.Mounts, mount{Kind: scratch, Path: at, InPlace: inPlace})
			continue
		case Private:
			if err := linkx.CloneTree(f.Shared, at); err != nil {
				return err
			}
			v.st.Mounts = append(v.st.Mounts, mount{Kind: scratch, Path: at, Target: f.Shared, InPlace: inPlace})
			continue
		}
		if only[f.Shared] && runtime.GOOS == "windows" {
			if err := linkx.Junction(at, f.Shared); err != nil {
				return err
			}
			v.st.Mounts = append(v.st.Mounts, mount{Kind: junction, Path: at, Target: f.Shared})
			continue
		}
		if err := linkx.CloneTree(f.Shared, at); err != nil {
			return err
		}
		m := mount{Kind: clone, Path: at, Target: f.Shared, Worlds: worldsOf(f.Rel), InPlace: inPlace}
		if err := a.snap(&m, r.Scratch, i+1); err != nil {
			return err
		}
		v.st.Mounts = append(v.st.Mounts, m)
	}
	return nil
}

func (a *Assembler) snap(m *mount, scratch string, i int) error {
	s, err := snapshot(m.Path, m.Skip)
	if err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	m.Snap = filepath.Join(scratch, "snap"+strconv.Itoa(i)+".json")
	return fsx.AtomicWrite(m.Snap, data, fsx.FilePerm)
}

// worldsOf says where worlds are in a shared folder: a shared saves folder
// holds them at its top.
func worldsOf(rel string) string {
	if rel == "saves" {
		return ""
	}
	return "-"
}

// Close merges the run's changes back and takes the view apart. It returns
// the worlds kept as copies because something else changed them too.
func (v *View) Close(_ context.Context) ([]Copy, error) {
	defer v.a.release(v.st.Leased)
	return v.st.close(v.a.c.Retitle)
}

func (st state) close(retitle func(dir, suffix string) error) ([]Copy, error) {
	var copies []Copy
	var errs []error
	// Shared folders first: their mount points sit inside the game folder.
	for i := len(st.Mounts) - 1; i >= 0; i-- {
		m := st.Mounts[i]
		var changes []change
		var err error
		switch m.Kind {
		case overlay:
			changes, err = upperChanges(m.Upper, m.Target)
		case clone:
			changes, err = cloneChanges(m)
		case junction:
			err = linkx.RemoveJunction(m.Path)
			if err == nil || errors.Is(err, fs.ErrNotExist) {
				err = os.MkdirAll(m.Path, 0o700)
			}
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(changes) > 0 {
			c, err := merger{target: m.Target, worlds: m.Worlds, start: st.Start, retitle: retitle}.apply(changes)
			copies = append(copies, c...)
			errs = append(errs, err)
		}
		if m.InPlace {
			errs = append(errs, os.RemoveAll(m.Path), os.MkdirAll(m.Path, 0o700))
		}
	}
	errs = append(errs, removeScratch(st.Scratch))
	return copies, errors.Join(errs...)
}

func cloneChanges(m mount) ([]change, error) {
	data, err := fsx.ReadCapped(m.Snap, 256<<20)
	if err != nil {
		return nil, err
	}
	var before map[string]stamp
	if err := json.Unmarshal(data, &before); err != nil {
		return nil, err
	}
	return diff(m.Path, before, m.Skip)
}

// removeScratch deletes a run's folder, first taking out any junction left
// in it, so deleting can never reach through one into shared data. Folders
// are made writable on the way: an overlay leaves its work folder at 000.
func removeScratch(dir string) error {
	if dir == "" {
		return nil
	}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		switch {
		case d == nil:
		case fsx.IsLink(d.Type()):
			if _, ok := linkx.JunctionTarget(p); ok {
				_ = linkx.RemoveJunction(p)
			}
		case d.IsDir():
			_ = os.Chmod(p, 0o700) //nolint:gosec // our own scratch folder, about to be deleted
		}
		return nil
	})
	return os.RemoveAll(dir)
}
