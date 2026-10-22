// Package screenshots lists the pictures in each instance's screenshots
// folder, remembers the world or server each was taken in (from the run that
// took it), re-saves new ones as JPEG when the player wants them smaller, and
// serves them through localimg tokens: full size for the viewer, a thumbnail
// for grids, the rotator and the filmstrip. Removing goes through holdbin,
// so it can be undone.
package screenshots

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/localimg"
	"quartz/internal/core/safe"
	"quartz/internal/holdbin"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

// ErrNoShot is returned for a screenshot that doesn't exist.
var ErrNoShot = errors.New("screenshots: no such screenshot")

var nameRe = regexp.MustCompile(`^[^/\\\x00-\x1f]{1,128}\.(?:png|PNG|jpg|jpeg)$`)

// ValidName reports whether name can be a screenshot file name.
func ValidName(name string) bool { return nameRe.MatchString(name) && !strings.HasPrefix(name, ".") }

// Shot is one screenshot.
type Shot struct {
	ID         string `json:"id"` // its file name
	InstanceID string `json:"instanceId"`
	World      string `json:"world"`
	Server     string `json:"server,omitempty"` // the server's name in the list, else its address
	Title      string `json:"title"`
	TakenAt    int64  `json:"takenAt"`
	File       string `json:"file"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Bytes      int64  `json:"bytes"`
	Image      string `json:"image"` // full size, on the image route
	Thumb      string `json:"thumb"` // a thumbnail
}

// Label is where a screenshot was taken: a world by its name, or a server
// by its address.
type Label struct {
	World  string `json:"world,omitempty"`
	Server string `json:"server,omitempty"`
}

// Deps wires a Service.
type Deps struct {
	Instances *instance.Store
	Images    *localimg.Registry
	Bus       *events.Bus
	Log       *slog.Logger
	Now       func() time.Time
	// ServerName is a server's name in the instance's list ("" if not there).
	ServerName func(instance, address string) string
	// Quality is the JPEG quality new screenshots are re-saved at; 0 keeps them.
	Quality func() int
}

// Service is safe for concurrent use.
type Service struct {
	d Deps

	mu     sync.Mutex
	dims   map[string]dims // path → size, valid while size and time are unchanged
	idx    sync.Mutex      // serialises shots.json writes
	shrink sync.Mutex      // one compression at a time (it is CPU-heavy)
}

type dims struct {
	w, h  int
	size  int64
	mtime time.Time
}

// New returns a Service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.ServerName == nil {
		d.ServerName = func(string, string) string { return "" }
	}
	if d.Quality == nil {
		d.Quality = func() int { return 0 }
	}
	return &Service{d: d, dims: map[string]dims{}}
}

func (s *Service) folder(id string) string {
	return s.d.Instances.Path(id, "screenshots")
}

// indexFile maps screenshot file names to where they were taken.
func (s *Service) indexFile(id string) string {
	return filepath.Join(s.d.Instances.Dir(id), "shots.json")
}

func (s *Service) index(id string) map[string]Label {
	m := map[string]Label{}
	if data, err := fsx.ReadCapped(s.indexFile(id), 4<<20); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

// editIndex changes an instance's index; s.idx serialises the writes.
func (s *Service) editIndex(id string, fn func(map[string]Label)) {
	s.idx.Lock()
	defer s.idx.Unlock()
	m := s.index(id)
	fn(m)
	if data, err := json.Marshal(m); err == nil {
		_ = fsx.AtomicWrite(s.indexFile(id), data, fsx.FilePerm)
	}
}

// sharing lists the instances whose screenshots are in dir (one, unless the
// folder is shared).
func (s *Service) sharing(dir string) []string {
	var out []string
	for _, in := range s.d.Instances.List() {
		if s.folder(in.ID) == dir {
			out = append(out, in.ID)
		}
	}
	return out
}

// labels maps the screenshots in id's folder to where they were taken. A
// shared folder holds every sharing instance's pictures, so their indexes
// count too; the instance's own wins a clash.
func (s *Service) labels(id string) map[string]Label {
	m := map[string]Label{}
	if dir := s.folder(id); dir != filepath.Join(s.d.Instances.GameDir(id), "screenshots") {
		for _, other := range s.sharing(dir) {
			if other != id {
				maps.Copy(m, s.index(other))
			}
		}
	}
	maps.Copy(m, s.index(id))
	return m
}

// List returns an instance's screenshots, newest first.
func (s *Service) List(id string) ([]Shot, error) {
	if root, err := os.OpenRoot(s.d.Instances.Base(id, "screenshots")); err == nil {
		holdbin.Purge(root, s.d.Now(), nil)
		_ = root.Close()
	}
	entries, err := os.ReadDir(s.folder(id))
	if errors.Is(err, os.ErrNotExist) {
		return []Shot{}, nil
	}
	if err != nil {
		return nil, err
	}
	labels := s.labels(id)
	servers := map[string]string{} // address → name in the list
	out := make([]Shot, 0, len(entries))
	for _, e := range entries {
		if !e.Type().IsRegular() || !ValidName(e.Name()) {
			continue
		}
		p := filepath.Join(s.folder(id), e.Name())
		fi, err := e.Info()
		if err != nil {
			continue
		}
		w, h := s.size(p, fi)
		token := s.d.Images.Register(p, fi)
		l := labels[e.Name()]
		server := ""
		if l.Server != "" {
			if _, ok := servers[l.Server]; !ok {
				servers[l.Server] = cmp.Or(s.d.ServerName(id, l.Server), l.Server)
			}
			server = servers[l.Server]
		}
		title := cmp.Or(l.World, server, "Screenshot")
		out = append(out, Shot{
			ID: e.Name(), InstanceID: id, World: l.World, Server: server, Title: title, TakenAt: fi.ModTime().UnixMilli(), File: e.Name(),
			Width: w, Height: h, Bytes: fi.Size(), Image: localimg.URL("shot", token), Thumb: localimg.URL("thumb", token),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.TakenAt != b.TakenAt {
			return b.TakenAt < a.TakenAt
		}
		return b.ID < a.ID
	})
	return out, nil
}

// size reads a picture's dimensions once per version of the file.
func (s *Service) size(p string, fi os.FileInfo) (int, int) {
	s.mu.Lock()
	d, ok := s.dims[p]
	s.mu.Unlock()
	if ok && d.size == fi.Size() && d.mtime.Equal(fi.ModTime()) {
		return d.w, d.h
	}
	w, h, _ := localimg.Dims(p)
	s.mu.Lock()
	s.dims[p] = dims{w: w, h: h, size: fi.Size(), mtime: fi.ModTime()}
	s.mu.Unlock()
	return w, h
}

// Recent returns the newest n screenshots of all instances.
func (s *Service) Recent(n int) []Shot {
	var all []Shot
	seen := map[string]bool{} // instances sharing screenshots list the same folder
	for _, in := range s.d.Instances.List() {
		if dir := s.folder(in.ID); !seen[dir] {
			seen[dir] = true
			if list, err := s.List(in.ID); err == nil {
				all = append(all, list...)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[j].TakenAt < all[i].TakenAt })
	return all[:min(n, len(all))]
}

// Path is a screenshot's file.
func (s *Service) Path(id, name string) (string, error) {
	if !ValidName(name) {
		return "", ErrNoShot
	}
	p := filepath.Join(s.folder(id), name)
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return "", ErrNoShot
	}
	return p, nil
}

// Trash moves screenshots aside and returns the token Undo takes.
func (s *Service) Trash(id string, names []string) (string, error) {
	var moves []holdbin.Move
	for _, n := range names {
		if _, err := s.Path(id, n); err != nil {
			return "", err
		}
		moves = append(moves, holdbin.Move{Rel: path.Join("screenshots", n)})
	}
	if len(moves) == 0 {
		return "", ErrNoShot
	}
	root, err := os.OpenRoot(s.d.Instances.Base(id, "screenshots"))
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	token, _, err := holdbin.Hold(root, moves, s.d.Now())
	if err != nil {
		return "", err
	}
	s.changed(id)
	return token, nil
}

// Undo brings back trashed screenshots.
func (s *Service) Undo(id, token string) error {
	root, err := os.OpenRoot(s.d.Instances.Base(id, "screenshots"))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	_, err = holdbin.Restore(root, token)
	s.changed(id)
	return err
}

// FinishRun counts the screenshots a run took and files each under the world
// or server it was in when the picture was taken, then re-saves them as JPEG
// when the player wants them smaller. It runs after worlds.Service.FinishRun,
// which names the world played.
func (s *Service) FinishRun(f *launch.Finished) {
	dir := s.folder(f.InstanceID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	taken := map[string]time.Time{}
	for _, e := range entries {
		if !e.Type().IsRegular() || !ValidName(e.Name()) {
			continue
		}
		if fi, err := e.Info(); err == nil && !fi.ModTime().Before(f.Started) && !fi.ModTime().After(f.Ended.Add(5*time.Second)) {
			taken[e.Name()] = fi.ModTime()
		}
	}
	if len(taken) == 0 {
		return
	}
	f.Record.Shots = len(taken)
	// A picture another run already filed (a shared folder) keeps its label.
	s.editIndex(f.InstanceID, func(m map[string]Label) {
		for n, at := range taken {
			if _, ok := m[n]; !ok {
				if l := labelAt(f, at); l != (Label{}) {
					m[n] = l
				}
			}
		}
	})
	s.changed(f.InstanceID)
	if q := s.d.Quality(); q > 0 {
		names := slices.Collect(maps.Keys(taken))
		safe.Go(s.d.Log, "compress screenshots", func() {
			if _, err := s.Compress(context.Background(), f.InstanceID, names, q); err != nil {
				s.d.Log.Warn("compressing screenshots failed", "instance", f.InstanceID, "err", err)
			}
		})
	}
}

// labelAt is where the run was when a picture was taken at t: the world or
// server it went into last before then (a second of slack for the log line
// coming after the click). Without a timeline (a run the launcher didn't
// watch) it is the world the run played, else the server it joined.
func labelAt(f *launch.Finished, t time.Time) Label {
	if len(f.Visits) == 0 {
		if f.Record.World != "" {
			return Label{World: f.Record.World}
		}
		return Label{Server: f.Record.Server}
	}
	var last *launch.Visit
	for i := range f.Visits {
		if !f.Visits[i].At.After(t.Add(time.Second)) {
			last = &f.Visits[i]
		}
	}
	switch {
	case last == nil:
		return Label{}
	case last.World:
		return Label{World: cmp.Or(last.Name, f.Record.World)}
	default:
		return Label{Server: last.Server}
	}
}

func (s *Service) changed(id string) {
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.ScreenshotsChanged, map[string]string{"instanceId": id})
	}
}
