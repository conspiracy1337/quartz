package syncing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/tasks"
	"quartz/internal/instance"
	"quartz/internal/settings"
)

// Errors returned to the UI.
var (
	ErrUnknown = errors.New("syncing: no such shared item")
	ErrRunning = errors.New("syncing: close the game first")
	ErrTaken   = errors.New("syncing: that path is already shared")
	ErrFull    = errors.New("syncing: too many custom paths")
)

// TrashKeep is how long a shared folder that was turned off stays restorable.
const TrashKeep = instance.TrashKeep

// Config wires the service.
type Config struct {
	Root      string // the shared folder
	Trash     string // the launcher's trash folder
	Settings  *settings.Store
	Instances *instance.Store
	Bus       *events.Bus
	// Running reports whether any copy of an instance is starting or running.
	Running func(id string) bool
	// HasMod reports whether any instance has one of these mods (by id).
	HasMod func(ids ...string) bool
	// Retitle renames a world whose folder got a suffix ("(2)").
	Retitle func(dir, suffix string) error
	// ClientJar is a Minecraft version's client jar (for keybind names).
	ClientJar func(mc string) string
	// ModJars lists an instance's enabled mod jars (for keybind names).
	ModJars func(id string) []ModJar
	// Tasks shows long moves on the Downloads page (nil runs them inline).
	Tasks *tasks.Engine
	Log   *slog.Logger
}

// Service is the shared layer.
type Service struct {
	c Config

	mu    sync.Mutex // settings and instance edits
	busy  sync.Mutex // guards locks
	locks map[string]*sync.Mutex
	lang  langCache
}

// ModJar is one enabled mod file of an instance.
type ModJar struct {
	Path string
	Name string
}

// New builds the service, installs its resolver into the instance store and
// drops joins of targets that are off (which have nothing to share).
func New(c Config) *Service {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	s := &Service{c: c, locks: map[string]*sync.Mutex{}}
	c.Instances.SetLocator(s.base)
	s.reconcile()
	return s
}

// Lock holds an instance's shared state still, for joins, leaves and the
// launch (which copies files in and assembles folders); the returned func
// releases it.
func (s *Service) Lock(id string) func() {
	s.busy.Lock()
	m, ok := s.locks[id]
	if !ok {
		m = &sync.Mutex{}
		s.locks[id] = m
	}
	s.busy.Unlock()
	m.Lock()
	return m.Unlock
}

func (s *Service) settings() settings.Syncing { return s.c.Settings.Get().Syncing }

func (s *Service) enabled(y settings.Syncing, id string) bool { return slices.Contains(y.Enabled, id) }

func (s *Service) running(id string) bool { return s.c.Running != nil && s.c.Running(id) }

// Joined lists the targets an instance shares now: joined and turned on.
func (s *Service) Joined(id string) []Target {
	in, err := s.c.Instances.Get(id)
	if err != nil {
		return nil
	}
	y := s.settings()
	var out []Target
	for _, t := range catalog(y) {
		if s.enabled(y, t.ID) && slices.Contains(in.Sync.Join, t.ID) {
			out = append(out, t)
		}
	}
	return out
}

// Folders lists the shared folders a run assembles, outermost only: a
// custom folder inside a shared folder is shared through it already.
func (s *Service) Folders(id string) []Target {
	var dirs []Target
	for _, t := range s.Joined(id) {
		if t.Dir {
			dirs = append(dirs, t)
		}
	}
	return slices.DeleteFunc(dirs, func(t Target) bool {
		return slices.ContainsFunc(dirs, func(o Target) bool { return o.ID != t.ID && o.covers(t.Path) })
	})
}

// SharedPath is where a target's one real copy lives.
func (s *Service) SharedPath(t Target) string {
	return filepath.Join(s.c.Root, filepath.FromSlash(t.Path))
}

// Root is the shared folder ("Open synced folder").
func (s *Service) Root() string { return s.c.Root }

// base is the instance store's Locator: the shared folder for a path inside
// a folder the instance shares, else "" (the game folder).
func (s *Service) base(id, rel string) string {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	for _, t := range s.Joined(id) {
		if t.Dir && t.covers(rel) {
			return s.c.Root
		}
	}
	return ""
}

// Busy reports whether files under rel can't change now: the instance runs,
// or it shares rel's folder with another instance that runs.
func (s *Service) Busy(id, rel string) bool {
	if s.running(id) {
		return true
	}
	if s.base(id, rel) == "" {
		return false
	}
	for _, in := range s.c.Instances.List() {
		if in.ID != id && s.running(in.ID) && s.base(in.ID, rel) != "" {
			return true
		}
	}
	return false
}

// reconcile keeps joins a subset of what is turned on.
func (s *Service) reconcile() {
	y := s.settings()
	for _, in := range s.c.Instances.List() {
		keep := slices.DeleteFunc(slices.Clone(in.Sync.Join), func(id string) bool { return !s.enabled(y, id) })
		if len(keep) != len(in.Sync.Join) {
			_, _ = s.c.Instances.Update(in.ID, func(i *instance.Instance) error { i.Sync.Join = keep; return nil })
		}
	}
}

func (s *Service) changed() {
	if s.c.Bus != nil {
		s.c.Bus.Emit(events.SyncChanged, nil)
		s.c.Bus.Emit(events.InstancesChanged, nil)
	}
}

// ---- the Syncing page ----

// Entry is one row of the Syncing page.
type Entry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"` // folders end in "/"
	Users     int    `json:"users"`
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
}

// Groups is the whole Syncing page.
type Groups struct {
	Files   []Entry `json:"files"`
	Folders []Entry `json:"folders"`
	Mods    []Entry `json:"mods"`
	Custom  []Entry `json:"custom"`
	Total   int     `json:"total"` // instances
}

// Groups lists every target with how many instances join it.
func (s *Service) Groups() Groups {
	y := s.settings()
	all := s.c.Instances.List()
	g := Groups{Total: len(all)}
	for _, t := range catalog(y) {
		e := Entry{ID: t.ID, Name: t.Name, Path: t.Path, Enabled: s.enabled(y, t.ID), Available: true}
		if t.Dir {
			e.Path += "/"
		}
		if t.Group == Custom {
			e.Name = ""
		}
		if len(t.ModIDs) > 0 && s.c.HasMod != nil {
			e.Available = s.c.HasMod(t.ModIDs...)
		}
		for _, in := range all {
			if slices.Contains(in.Sync.Join, t.ID) {
				e.Users++
			}
		}
		switch t.Group {
		case Files:
			g.Files = append(g.Files, e)
		case Folders:
			g.Folders = append(g.Folders, e)
		case Mods:
			g.Mods = append(g.Mods, e)
		case Custom:
			g.Custom = append(g.Custom, e)
		}
	}
	return g
}

// SetEnabled turns a target on or off. On costs nothing until an instance
// joins; off makes every joined instance leave (each keeps a copy) and moves
// the shared copy to the trash.
func (s *Service) SetEnabled(ctx context.Context, id string, on bool) error {
	t, ok := find(s.settings(), id)
	if !ok {
		return ErrUnknown
	}
	if on {
		s.editSettings(func(y *settings.Syncing) {
			if !slices.Contains(y.Enabled, id) {
				y.Enabled = append(y.Enabled, id)
			}
		})
		s.changed()
		return nil
	}
	users := s.users(id)
	for _, in := range users {
		if s.running(in.ID) {
			return ErrRunning
		}
	}
	for _, in := range users {
		if err := s.Leave(ctx, in.ID, id); err != nil {
			return err
		}
	}
	if err := s.trashShared(t); err != nil {
		return err
	}
	s.editSettings(func(y *settings.Syncing) {
		y.Enabled = slices.DeleteFunc(y.Enabled, func(e string) bool { return e == id })
	})
	s.changed()
	return nil
}

func (s *Service) users(id string) []instance.Instance {
	return slices.DeleteFunc(s.c.Instances.List(), func(in instance.Instance) bool { return !slices.Contains(in.Sync.Join, id) })
}

func (s *Service) editSettings(fn func(*settings.Syncing)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.c.Settings.Get()
	y := st.Syncing
	y.Enabled, y.Custom, y.Keys = slices.Clone(y.Enabled), slices.Clone(y.Custom), slices.Clone(y.Keys)
	fn(&y)
	st.Syncing = y
	s.c.Settings.Replace(st)
}

// AddCustom shares a path inside the instance folder, turned on.
func (s *Service) AddCustom(path string, dir bool) (Entry, error) {
	p, err := settings.CleanSyncPath(path)
	if err != nil {
		return Entry{}, err
	}
	y := s.settings()
	if len(y.Custom) >= 64 {
		return Entry{}, ErrFull
	}
	probe := Target{Path: p, Dir: dir}
	for _, t := range catalog(y) {
		if t.Path == p || (t.Group == Custom && (t.covers(p) || probe.covers(t.Path))) {
			return Entry{}, ErrTaken
		}
	}
	var b [3]byte
	_, _ = rand.Read(b[:])
	c := settings.SyncCustom{ID: "c-" + hex.EncodeToString(b[:]), Path: p, Dir: dir}
	s.editSettings(func(y *settings.Syncing) {
		y.Custom = append(y.Custom, c)
		y.Enabled = append(y.Enabled, c.ID)
	})
	s.changed()
	e := Entry{ID: c.ID, Path: p, Enabled: true, Available: true}
	if dir {
		e.Path += "/"
	}
	return e, nil
}

// RemoveCustom stops sharing a custom path: every joined instance keeps a
// copy, and the shared copy goes to the trash.
func (s *Service) RemoveCustom(ctx context.Context, id string) error {
	t, ok := find(s.settings(), id)
	if !ok || t.Group != Custom {
		return ErrUnknown
	}
	if err := s.SetEnabled(ctx, id, false); err != nil {
		return err
	}
	s.editSettings(func(y *settings.Syncing) {
		y.Custom = slices.DeleteFunc(y.Custom, func(c settings.SyncCustom) bool { return c.ID == id })
	})
	s.changed()
	return nil
}

// trashShared moves a target's shared copy to trash/shared/<id>~<ms>.
func (s *Service) trashShared(t Target) error {
	src := s.SharedPath(t)
	if _, err := os.Lstat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	dst := filepath.Join(s.c.Trash, "shared", t.ID+"~"+strconv.FormatInt(time.Now().UnixMilli(), 10))
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	return os.Rename(src, filepath.Join(dst, filepath.Base(src)))
}

// PurgeTrash deletes shared folders turned off more than TrashKeep ago.
func (s *Service) PurgeTrash(now time.Time) {
	dir := filepath.Join(s.c.Trash, "shared")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		_, ms, ok := strings.Cut(e.Name(), "~")
		at, err := strconv.ParseInt(ms, 10, 64)
		if !ok || err != nil || now.Sub(time.UnixMilli(at)) <= TrashKeep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			s.c.Log.Warn("purging a shared folder failed", "folder", e.Name(), "err", err)
		}
	}
}

// ---- joining and leaving ----

// JoinAll joins a new instance to the targets New instance picked (turning
// each on) and to the shared keybinds for KeysID.
func (s *Service) JoinAll(ctx context.Context, id string, targets []string) error {
	var errs []error
	for _, t := range targets {
		if t == KeysID {
			errs = append(errs, s.SetKeys(id, true))
			continue
		}
		if _, ok := find(s.settings(), t); !ok {
			continue
		}
		if err := s.SetEnabled(ctx, t, true); err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, s.Join(ctx, id, t))
	}
	return errors.Join(errs...)
}

// SetKeys turns an instance's shared keybinds on or off.
func (s *Service) SetKeys(id string, on bool) error {
	_, err := s.c.Instances.Update(id, func(in *instance.Instance) error { in.Sync.Keys = on; return nil })
	if err == nil {
		s.changed()
	}
	return err
}

// Join makes an instance share a target that is turned on. The instance's
// own items move into the shared copy first, so nothing it had disappears.
func (s *Service) Join(ctx context.Context, id, target string) error {
	y := s.settings()
	t, ok := find(y, target)
	if !ok || !s.enabled(y, target) {
		return ErrUnknown
	}
	if s.running(id) {
		return ErrRunning
	}
	unlock := s.Lock(id)
	defer unlock()
	in, err := s.c.Instances.Get(id)
	if err != nil {
		return err
	}
	if slices.Contains(in.Sync.Join, target) {
		return nil
	}
	if !s.coveredByOther(id, t) {
		game := filepath.Join(s.c.Instances.GameDir(id), filepath.FromSlash(t.Path))
		if t.Dir {
			if err := s.move(ctx, in, t, game, s.SharedPath(t), false); err != nil {
				return err
			}
		} else if err := joinFile(t, game, s.SharedPath(t)); err != nil {
			return err
		}
	}
	return s.setJoined(id, target, true)
}

// Leave stops an instance sharing a target; it keeps its own copy.
func (s *Service) Leave(ctx context.Context, id, target string) error {
	t, ok := find(s.settings(), target)
	if !ok {
		return ErrUnknown
	}
	if s.running(id) {
		return ErrRunning
	}
	unlock := s.Lock(id)
	defer unlock()
	in, err := s.c.Instances.Get(id)
	if err != nil {
		return err
	}
	if !slices.Contains(in.Sync.Join, target) {
		return nil
	}
	if !s.coveredByOther(id, t) {
		game := filepath.Join(s.c.Instances.GameDir(id), filepath.FromSlash(t.Path))
		if t.Dir {
			if err := s.move(ctx, in, t, s.SharedPath(t), game, true); err != nil {
				return err
			}
			s.clearInner(id, t)
		} else if t.ID != OptionsID { // options are merged into the instance's own file at every launch
			if err := leaveFile(game, s.SharedPath(t)); err != nil {
				return err
			}
		}
	}
	return s.setJoined(id, target, false)
}

// coveredByOther reports whether another joined folder already shares t's
// path, so joining or leaving t moves nothing.
func (s *Service) coveredByOther(id string, t Target) bool {
	for _, o := range s.Joined(id) {
		if o.ID != t.ID && o.Dir && o.covers(t.Path) {
			return true
		}
	}
	return false
}

// clearInner empties the copies of folders the instance still shares inside
// one it just left: they stay shared, so the copy would only hide behind
// them (and clash on the next join).
func (s *Service) clearInner(id string, left Target) {
	game := s.c.Instances.GameDir(id)
	for _, o := range s.Joined(id) {
		if o.ID != left.ID && left.covers(o.Path) {
			p := filepath.Join(game, filepath.FromSlash(o.Path))
			if fsx.Inside(game, p) {
				_ = os.RemoveAll(p)
				_ = os.MkdirAll(p, 0o700)
			}
		}
	}
}

func (s *Service) setJoined(id, target string, on bool) error {
	_, err := s.c.Instances.Update(id, func(in *instance.Instance) error {
		in.Sync.Join = slices.DeleteFunc(slices.Clone(in.Sync.Join), func(j string) bool { return j == target })
		if on {
			in.Sync.Join = append(in.Sync.Join, target)
		}
		return nil
	})
	if err == nil {
		s.changed()
	}
	return err
}

// move places every item of from into to (keep copies instead of moving),
// as a job when there is an engine, so a big move shows its progress.
func (s *Service) move(ctx context.Context, in instance.Instance, t Target, from, to string, keep bool) error {
	items, err := plan(from, to)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(to, 0o700); err != nil {
		return err
	}
	place := func(it item) error {
		if err := it.apply(keep); err != nil {
			return fmt.Errorf("syncing: %s: %w", filepath.Base(it.from), err)
		}
		if it.suffix != "" && t.ID == "saves" && s.c.Retitle != nil {
			_ = s.c.Retitle(it.to, it.suffix)
		}
		return nil
	}
	if s.c.Tasks == nil || len(items) == 0 {
		for _, it := range items {
			if err := place(it); err != nil {
				return err
			}
		}
		return nil
	}
	works := make([]tasks.Work, 0, len(items))
	for _, it := range items {
		works = append(works, tasks.Func(filepath.Base(it.from), func(context.Context) error { return place(it) }))
	}
	title := "Sharing " + t.Name
	if keep {
		title = "Copying " + t.Name
	}
	spec := tasks.NewJob(tasks.Meta{Title: title, Subtitle: in.Name, Art: tasks.Art{Background: in.Art.Background, Initial: in.Art.Initial}, Kind: "sync"}).
		Step("Moving files", tasks.Count, works...)
	job, err := s.c.Tasks.Submit(spec)
	if err != nil {
		return err
	}
	return s.c.Tasks.Wait(ctx, job)
}
