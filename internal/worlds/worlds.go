// Package worlds lists and changes an instance's singleplayer worlds: the
// folders in saves/ with a level.dat. It reads level.dat (NBT) for the name,
// game mode, version and seed, imports worlds from zips or folders, exports
// them as zips, copies them to other instances, renames them in level.dat and
// removes them with Undo. Changes are refused while the instance's game runs,
// since the game rewrites level.dat on exit.
package worlds

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"quartz/internal/backup"
	"quartz/internal/core/archive"
	"quartz/internal/core/events"
	"quartz/internal/core/linkx"
	"quartz/internal/core/localimg"
	"quartz/internal/core/nbt"
	"quartz/internal/holdbin"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

// Errors returned to the UI.
var (
	ErrBusy     = errors.New("close the game first")
	ErrNoWorld  = errors.New("worlds: there is no such world")
	ErrNotWorld = errors.New("that isn't a Minecraft world (it has no level.dat)")
	ErrName     = errors.New("worlds: enter a name of 1 to 64 characters")
)

// World is one singleplayer world.
type World struct {
	Folder     string `json:"folder"` // its folder in saves/, which Quick Play opens
	Name       string `json:"name"`   // LevelName
	Mode       string `json:"mode"`   // Survival | Creative | Adventure | Spectator | Hardcore
	LastPlayed int64  `json:"lastPlayed"`
	Version    string `json:"version,omitempty"`
	Bytes      int64  `json:"bytes"`
	Icon       string `json:"icon,omitempty"` // the world's icon.png on the image route
}

// Deps wires a Service.
type Deps struct {
	Instances *instance.Store
	Images    *localimg.Registry
	// Busy reports an instance that is starting or running.
	Busy func(id string) bool
	Bus  *events.Bus
	Log  *slog.Logger
	Now  func() time.Time
}

// Service is safe for concurrent use.
type Service struct {
	d Deps

	mu    sync.Mutex
	sizes map[string]sized // world folder → size, valid while level.dat is unchanged
}

type sized struct {
	mod   time.Time
	bytes int64
}

// New returns a Service.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Busy == nil {
		d.Busy = func(string) bool { return false }
	}
	return &Service{d: d, sizes: map[string]sized{}}
}

func (s *Service) saves(id string) string { return filepath.Join(s.d.Instances.GameDir(id), "saves") }

// List returns an instance's worlds, the most recently played first.
func (s *Service) List(ctx context.Context, id string) ([]World, error) {
	s.purge(id)
	saves := s.saves(id)
	entries, err := os.ReadDir(saves)
	if errors.Is(err, fs.ErrNotExist) {
		return []World{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]World, 0, len(entries))
	var mu sync.Mutex
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for _, e := range entries {
		if !e.IsDir() || !instance.ValidWorldFolder(e.Name()) {
			continue
		}
		dir := filepath.Join(saves, e.Name())
		g.Go(func() error {
			w, ok := s.read(dir)
			if ok {
				mu.Lock()
				out = append(out, w)
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	slices.SortFunc(out, func(a, b World) int {
		return cmp.Or(cmp.Compare(b.LastPlayed, a.LastPlayed), strings.Compare(a.Folder, b.Folder))
	})
	return out, nil
}

// Count is how many worlds an instance has, without reading them.
func (s *Service) Count(id string) int {
	entries, err := os.ReadDir(s.saves(id))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && instance.ValidWorldFolder(e.Name()) {
			if _, err := os.Lstat(filepath.Join(s.saves(id), e.Name(), "level.dat")); err == nil {
				n++
			}
		}
	}
	return n
}

// read describes one world folder; false when it holds no level.dat.
func (s *Service) read(dir string) (World, bool) {
	lvl := filepath.Join(dir, "level.dat")
	fi, err := os.Lstat(lvl)
	if err != nil || !fi.Mode().IsRegular() {
		return World{}, false
	}
	w := World{Folder: filepath.Base(dir), Name: filepath.Base(dir), LastPlayed: fi.ModTime().UnixMilli()}
	if data := levelData(dir); data != nil {
		if n := data.Get("LevelName").Text(); n != "" {
			w.Name = n
		}
		w.Mode = mode(data)
		if lp := data.Get("LastPlayed").Int(); lp > 0 {
			w.LastPlayed = lp
		}
		w.Version = data.Path("Version", "Name").Text()
	}
	if ic, err := os.Lstat(filepath.Join(dir, "icon.png")); err == nil && ic.Mode().IsRegular() && s.d.Images != nil {
		w.Icon = localimg.URL("world", s.d.Images.Register(filepath.Join(dir, "icon.png"), ic))
	}
	w.Bytes = s.size(dir, fi.ModTime())
	return w, true
}

// levelData reads level.dat's Data compound (level.dat_old as a fallback).
func levelData(dir string) *nbt.Tag {
	for _, name := range []string{"level.dat", "level.dat_old"} {
		if f, err := nbt.ReadFile(filepath.Join(dir, name)); err == nil {
			if d := f.Root.Get("Data"); d != nil && d.Kind == nbt.Compound {
				return d
			}
		}
	}
	return nil
}

func mode(data *nbt.Tag) string {
	if data.Get("hardcore").Int() != 0 {
		return "Hardcore"
	}
	switch data.Get("GameType").Int() {
	case 1:
		return "Creative"
	case 2:
		return "Adventure"
	case 3:
		return "Spectator"
	}
	return "Survival"
}

// size is a world folder's size, walked once per level.dat change (the
// game rewrites level.dat whenever it saves).
func (s *Service) size(dir string, mod time.Time) int64 {
	s.mu.Lock()
	c, ok := s.sizes[dir]
	s.mu.Unlock()
	if ok && c.mod.Equal(mod) {
		return c.bytes
	}
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped
		}
		if d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	s.mu.Lock()
	s.sizes[dir] = sized{mod: mod, bytes: n}
	s.mu.Unlock()
	return n
}

// dir returns a world's folder after checking its name.
func (s *Service) dir(id, folder string) (string, error) {
	if !instance.ValidWorldFolder(folder) {
		return "", ErrNoWorld
	}
	d := filepath.Join(s.saves(id), folder)
	if fi, err := os.Lstat(filepath.Join(d, "level.dat")); err != nil || !fi.Mode().IsRegular() {
		return "", ErrNoWorld
	}
	return d, nil
}

// Path is a world's folder, for Show in folder.
func (s *Service) Path(id, folder string) (string, error) { return s.dir(id, folder) }

// Seed returns a world's seed as decimal text (it doesn't fit a JS number).
func (s *Service) Seed(id, folder string) (string, error) {
	d, err := s.dir(id, folder)
	if err != nil {
		return "", err
	}
	data := levelData(d)
	if data == nil {
		return "", ErrNotWorld
	}
	if seed := data.Path("WorldGenSettings", "seed"); seed != nil {
		return strconv.FormatInt(seed.Int(), 10), nil
	}
	return strconv.FormatInt(data.Get("RandomSeed").Int(), 10), nil
}

// Rename changes the name the game shows (LevelName); the folder stays.
func (s *Service) Rename(id, folder, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 || strings.ContainsAny(name, "\x00\n\r") {
		return ErrName
	}
	if s.d.Busy(id) {
		return ErrBusy
	}
	d, err := s.dir(id, folder)
	if err != nil {
		return err
	}
	lvl := filepath.Join(d, "level.dat")
	f, err := nbt.ReadFile(lvl)
	if err != nil {
		return err
	}
	data := f.Root.Get("Data")
	if data == nil || data.Kind != nbt.Compound {
		return ErrNotWorld
	}
	data.Comp.Set("LevelName", nbt.NewString(name))
	if err := nbt.WriteFile(lvl, f); err != nil {
		return err
	}
	s.changed(id)
	return nil
}

// Delete moves a world aside and returns the token Undo takes.
func (s *Service) Delete(id, folder string) (string, error) {
	if s.d.Busy(id) {
		return "", ErrBusy
	}
	if _, err := s.dir(id, folder); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(s.d.Instances.GameDir(id))
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	token, _, err := holdbin.Hold(root, []holdbin.Move{{Rel: path.Join("saves", folder)}}, s.d.Now())
	if err != nil {
		return "", err
	}
	s.changed(id)
	return token, nil
}

// Undo brings back a deleted world.
func (s *Service) Undo(id, token string) error {
	root, err := os.OpenRoot(s.d.Instances.GameDir(id))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	_, err = holdbin.Restore(root, token)
	s.changed(id)
	return err
}

// purge deletes worlds whose Undo time is over.
func (s *Service) purge(id string) {
	if root, err := os.OpenRoot(s.d.Instances.GameDir(id)); err == nil {
		holdbin.Purge(root, s.d.Now(), nil)
		_ = root.Close()
	}
}

// Export zips a world to dest (a path the player chose in a save dialog).
// The zip holds the world's folder, so it imports anywhere.
func (s *Service) Export(id, folder, dest string) error {
	d, err := s.dir(id, folder)
	if err != nil {
		return err
	}
	if err := backup.ZipDir(d, dest, folder+"/"); err != nil {
		if errors.Is(err, backup.ErrNoWorlds) {
			return ErrNoWorld
		}
		return err
	}
	return nil
}

// CopyTo copies a world into another instance and returns its folder there.
func (s *Service) CopyTo(id, folder, target string) (string, error) {
	if s.d.Busy(id) {
		return "", ErrBusy
	}
	d, err := s.dir(id, folder)
	if err != nil {
		return "", err
	}
	if _, err := s.d.Instances.Get(target); err != nil {
		return "", err
	}
	dest := s.saves(target)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return "", err
	}
	name := free(dest, folder)
	// A world is edited in place, so the copy is never a hardlink.
	if err := linkx.CopyTree(d, filepath.Join(dest, name), linkx.Options{NoHardlink: true}); err != nil {
		return "", err
	}
	s.changed(target)
	return name, nil
}

// Import adds a world from a .zip or a world folder (a path from a native
// dialog or a drop, never from the page) and returns its folder.
func (s *Service) Import(id, src string) (string, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	saves := s.saves(id)
	if err := os.MkdirAll(saves, 0o700); err != nil {
		return "", err
	}
	if fi.IsDir() {
		if _, err := os.Lstat(filepath.Join(src, "level.dat")); err != nil {
			return "", ErrNotWorld
		}
		name := free(saves, folderName(filepath.Base(src)))
		if err := linkx.CopyTree(src, filepath.Join(saves, name), linkx.Options{NoHardlink: true}); err != nil {
			return "", err
		}
		s.changed(id)
		return name, nil
	}
	prefix, err := worldPrefix(src)
	if err != nil {
		return "", err
	}
	base := path.Base(strings.TrimSuffix(prefix, "/"))
	if prefix == "" {
		base = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	}
	// Unpack beside saves/ first, so a failed import leaves nothing behind.
	staging := filepath.Join(s.d.Instances.GameDir(id), ".quartz", "import-"+random())
	defer func() { _ = os.RemoveAll(staging) }()
	if err := archive.ExtractZip(src, staging, archive.Options{Prefix: prefix}); err != nil {
		return "", err
	}
	if _, err := os.Lstat(filepath.Join(staging, "level.dat")); err != nil {
		return "", ErrNotWorld
	}
	name := free(saves, folderName(base))
	if err := os.Rename(staging, filepath.Join(saves, name)); err != nil {
		return "", err
	}
	s.changed(id)
	return name, nil
}

// worldPrefix finds the folder inside a zip that holds level.dat: the zip's
// root, or one or two folders down ("World/", "saves/World/").
func worldPrefix(zipPath string) (string, error) {
	r, err := archive.Open(zipPath)
	if err != nil {
		return "", ErrNotWorld
	}
	defer func() { _ = r.Close() }()
	best := -1
	prefix := ""
	for _, e := range r.Entries("") {
		if path.Base(e.Name) != "level.dat" || !e.Regular {
			continue
		}
		depth := strings.Count(e.Name, "/")
		if depth > 2 || (best >= 0 && depth >= best) {
			continue
		}
		best, prefix = depth, strings.TrimSuffix(e.Name, "level.dat")
	}
	if best < 0 {
		return "", ErrNotWorld
	}
	return prefix, nil
}

// folderName turns a name into a usable world folder name.
func folderName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
	s = strings.TrimLeft(s, ".")
	if len(s) > 100 {
		s = s[:100]
	}
	if !instance.ValidWorldFolder(s) {
		return "World"
	}
	return s
}

// free returns name, or "name (2)", "name (3)" … when it is taken in dir.
func free(dir, name string) string {
	try := name
	for n := 2; ; n++ {
		if _, err := os.Lstat(filepath.Join(dir, try)); errors.Is(err, fs.ErrNotExist) {
			return try
		}
		try = name + " (" + strconv.Itoa(n) + ")"
	}
}

func random() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Service) changed(id string) {
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.WorldsChanged, map[string]string{"instanceId": id})
	}
}

// FinishRun names the world a run played: the one whose level.dat changed
// last while the game ran.
func (s *Service) FinishRun(f *launch.Finished) {
	saves := filepath.Join(f.GameDir, "saves")
	entries, err := os.ReadDir(saves)
	if err != nil {
		return
	}
	var latest time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		fi, err := os.Lstat(filepath.Join(saves, e.Name(), "level.dat"))
		if err != nil || fi.ModTime().Before(f.Started) || !fi.ModTime().After(latest) {
			continue
		}
		latest = fi.ModTime()
		f.Record.World = e.Name()
		if data := levelData(filepath.Join(saves, e.Name())); data != nil {
			if n := data.Get("LevelName").Text(); n != "" {
				f.Record.World = n
			}
		}
	}
	if !latest.IsZero() {
		s.changed(f.InstanceID)
	}
}
