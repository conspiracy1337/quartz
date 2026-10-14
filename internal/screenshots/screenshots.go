// Package screenshots lists the pictures in each instance's screenshots
// folder, remembers which world each was taken in (from the run that took
// it), and serves them through localimg tokens: full size for the viewer, a
// thumbnail for grids, the rotator and the filmstrip. Removing goes through
// holdbin, so it can be undone.
package screenshots

import (
	"cmp"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/localimg"
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
	Title      string `json:"title"`
	TakenAt    int64  `json:"takenAt"`
	File       string `json:"file"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Bytes      int64  `json:"bytes"`
	Image      string `json:"image"` // full size, on the image route
	Thumb      string `json:"thumb"` // a thumbnail
}

// Deps wires a Service.
type Deps struct {
	Instances *instance.Store
	Images    *localimg.Registry
	Bus       *events.Bus
	Now       func() time.Time
}

// Service is safe for concurrent use.
type Service struct {
	d Deps

	mu   sync.Mutex
	dims map[string]dims // path → size, valid while size and time are unchanged
	idx  sync.Mutex      // serialises shots.json writes
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
	return &Service{d: d, dims: map[string]dims{}}
}

func (s *Service) folder(id string) string {
	return s.d.Instances.Path(id, "screenshots")
}

// indexFile maps screenshot file names to the world they were taken in.
func (s *Service) indexFile(id string) string {
	return filepath.Join(s.d.Instances.Dir(id), "shots.json")
}

func (s *Service) index(id string) map[string]string {
	m := map[string]string{}
	if data, err := fsx.ReadCapped(s.indexFile(id), 4<<20); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

// worlds maps the screenshots in id's folder to their worlds. A shared
// folder holds every sharing instance's pictures, so their indexes count
// too; the instance's own wins a clash.
func (s *Service) worlds(id string) map[string]string {
	m := map[string]string{}
	if dir := s.folder(id); dir != filepath.Join(s.d.Instances.GameDir(id), "screenshots") {
		for _, in := range s.d.Instances.List() {
			if in.ID != id && s.folder(in.ID) == dir {
				maps.Copy(m, s.index(in.ID))
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
	worlds := s.worlds(id)
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
		world := worlds[e.Name()]
		title := world
		if title == "" {
			title = "Screenshot"
		}
		out = append(out, Shot{
			ID: e.Name(), InstanceID: id, World: world, Title: title, TakenAt: fi.ModTime().UnixMilli(), File: e.Name(),
			Width: w, Height: h, Bytes: fi.Size(), Image: localimg.URL("shot", token), Thumb: localimg.URL("thumb", token),
		})
	}
	slices.SortFunc(out, func(a, b Shot) int { return cmp.Or(cmp.Compare(b.TakenAt, a.TakenAt), strings.Compare(b.ID, a.ID)) })
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
	slices.SortStableFunc(all, func(a, b Shot) int { return cmp.Compare(b.TakenAt, a.TakenAt) })
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

// FinishRun counts the screenshots a run took and files them under the
// world it played. It runs after worlds.Service.FinishRun, which names it.
func (s *Service) FinishRun(f *launch.Finished) {
	dir := s.folder(f.InstanceID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var taken []string
	for _, e := range entries {
		if !e.Type().IsRegular() || !ValidName(e.Name()) {
			continue
		}
		if fi, err := e.Info(); err == nil && !fi.ModTime().Before(f.Started) && !fi.ModTime().After(f.Ended.Add(5*time.Second)) {
			taken = append(taken, e.Name())
		}
	}
	if len(taken) == 0 {
		return
	}
	f.Record.Shots = len(taken)
	if f.Record.World != "" {
		s.idx.Lock()
		m := s.index(f.InstanceID)
		for _, n := range taken {
			m[n] = f.Record.World
		}
		if data, err := json.Marshal(m); err == nil {
			_ = fsx.AtomicWrite(s.indexFile(f.InstanceID), data, fsx.FilePerm)
		}
		s.idx.Unlock()
	}
	s.changed(f.InstanceID)
}

func (s *Service) changed(id string) {
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.ScreenshotsChanged, map[string]string{"instanceId": id})
	}
}
