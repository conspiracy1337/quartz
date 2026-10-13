package shortcuts

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"quartz/internal/core/jsonfile"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/intent"
)

// ErrNoLauncher means the launcher's own path is unknown.
var ErrNoLauncher = errors.New("shortcuts: can't find the launcher's program file")

// Service makes and maintains the shortcuts recorded on instances.
type Service struct {
	w        writer
	dir      string // icons the shortcuts refer to, and state.json
	store    *instance.Store
	pictures *icons.Library
	exe      string
	log      *slog.Logger

	mu sync.Mutex // one shortcut change at a time
}

// New returns the service; dir is <data>/shortcuts, exe the launcher.
func New(dir string, store *instance.Store, pictures *icons.Library, exe string, log *slog.Logger) *Service {
	return &Service{w: newWriter(), dir: dir, store: store, pictures: pictures, exe: exe, log: log}
}

// Create makes a shortcut in each place and records them on the instance.
// account ("" for the default) and into are checked by the caller.
func (s *Service) Create(id string, where []Where, account string, into instance.Into) ([]string, error) {
	if s.exe == "" {
		return nil, ErrNoLauncher
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	in, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if len(in.Shortcuts)+len(where) > instance.MaxShortcuts {
		return nil, errors.New("shortcuts: this instance has enough shortcuts already")
	}
	pic, err := Picture(in, s.pictures)
	if err != nil {
		return nil, err
	}
	var made []instance.Shortcut
	var paths []string
	for _, w := range where {
		rec := instance.Shortcut{Where: string(w), Account: account, Into: into}
		p, err := s.w.create(s.spec(in, rec, pic), s.dir)
		if err != nil {
			s.removeAll(in.ID, made)
			return nil, err
		}
		rec.Path = p
		made = append(made, rec)
		paths = append(paths, p)
	}
	_, err = s.store.Update(id, func(i *instance.Instance) error {
		i.Shortcuts = append(i.Shortcuts, made...)
		return nil
	})
	return paths, err
}

func (s *Service) spec(in instance.Instance, rec instance.Shortcut, pic []byte) Spec {
	l := intent.Launch{Instance: in.ID, Account: rec.Account, Into: rec.Into}
	return Spec{ID: in.ID, Name: in.Name, Exe: s.exe, Args: l.Argv(), Icon: pic, Where: Where(rec.Where)}
}

func (s *Service) removeAll(id string, recs []instance.Shortcut) {
	for _, r := range recs {
		if r.Path == "" {
			continue
		}
		if err := s.w.remove(r.Path, id); err != nil {
			s.log.Warn("removing a shortcut failed", "path", r.Path, "err", err)
		}
	}
}

// Hide removes the files of a trashed instance's shortcuts; its records stay
// in the trashed instance.json for Restore.
func (s *Service) Hide(in instance.Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeAll(in.ID, in.Shortcuts)
	for _, ext := range []string{".png", ".ico"} {
		_ = os.Remove(filepath.Join(s.dir, in.ID+ext))
	}
}

// Restore recreates a restored instance's shortcuts (its id may have
// changed) and records where they now are.
func (s *Service) Restore(in instance.Instance) {
	if len(in.Shortcuts) == 0 || s.exe == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rewrite(in)
}

// rewrite writes every recorded shortcut of in again; s.mu is held.
func (s *Service) rewrite(in instance.Instance) {
	pic, err := Picture(in, s.pictures)
	if err != nil {
		s.log.Warn("drawing a shortcut icon failed", "instance", in.ID, "err", err)
		return
	}
	recs := slices.Clone(in.Shortcuts)
	for i, r := range recs {
		if r.Path != "" {
			_ = s.w.remove(r.Path, in.ID)
		}
		p, err := s.w.create(s.spec(in, r, pic), s.dir)
		if err != nil {
			s.log.Warn("writing a shortcut failed", "instance", in.ID, "err", err)
			p = ""
		}
		recs[i].Path = p
	}
	recs = slices.DeleteFunc(recs, func(r instance.Shortcut) bool { return r.Path == "" })
	if _, err := s.store.Update(in.ID, func(i *instance.Instance) error { i.Shortcuts = recs; return nil }); err != nil {
		s.log.Warn("recording shortcuts failed", "instance", in.ID, "err", err)
	}
}

type state struct {
	Exe string `json:"exe"`
}

// Refresh rewrites every shortcut when the launcher's path changed since the
// last run (it was moved or updated in another place). Unchanged starts only
// read one small file.
func (s *Service) Refresh() {
	if s.exe == "" {
		return
	}
	opts := jsonfile.Options[state]{Path: filepath.Join(s.dir, "state.json"), Default: func() state { return state{} }}
	was, _ := jsonfile.Read(opts)
	if was.Exe == s.exe {
		return
	}
	s.mu.Lock()
	for _, in := range s.store.List() {
		if len(in.Shortcuts) > 0 {
			s.rewrite(in)
		}
	}
	s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return
	}
	if st, err := jsonfile.Open(opts); err == nil {
		st.Set(state{Exe: s.exe})
		_ = st.Flush()
	}
}
