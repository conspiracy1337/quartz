package syncing

import (
	"context"
	"path/filepath"
	"time"

	"quartz/internal/core/keycodes"
	"quartz/internal/core/optfile"
	"quartz/internal/settings"
)

// Session is one launch's share of the files: Prepare copies the shared
// files in and writes the shared keybinds, Finish copies what the run
// changed back out. Folders are not its business; the run view assembles
// them (see Folders).
type Session struct {
	s       *Service
	id      string
	game    string
	files   []Target
	options bool
	before  map[string]string    // the instance's options at launch, shared form
	stamps  map[string]time.Time // each synced file's time at launch
}

const optionsFile = "options.txt"

// Prepare readies an instance's files for a launch. Call it before the run
// view is assembled, so a private copy starts from the merged files.
func (s *Service) Prepare(id string) (*Session, error) {
	in, err := s.c.Instances.Get(id)
	if err != nil {
		return nil, err
	}
	unlock := s.Lock(id)
	defer unlock()
	se := &Session{s: s, id: id, game: s.c.Instances.GameDir(id), stamps: map[string]time.Time{}}
	// A shared folder's place in the instance must be empty for the run to
	// assemble it; anything that appeared there joins the shared copy.
	for _, t := range s.Folders(id) {
		if err := s.move(context.Background(), in, t, se.path(t), s.SharedPath(t), false); err != nil {
			return nil, err
		}
	}
	for _, t := range s.Joined(id) {
		switch {
		case t.Dir:
		case t.ID == OptionsID:
			se.options = true
		default:
			se.files = append(se.files, t)
			if err := newest(se.path(t), s.SharedPath(t)); err != nil {
				return nil, err
			}
			se.stamps[t.ID] = stamp(se.path(t))
		}
	}
	keys := s.settings().Keys
	if !se.options && (!in.Sync.Keys || len(keys) == 0) {
		return se, nil
	}
	f, err := optfile.Read(se.optionsPath(), ":")
	if err != nil {
		return nil, err
	}
	fm := formatOf(in.Minecraft, f)
	changed := false
	if se.options {
		shared, err := optfile.Read(filepath.Join(s.c.Root, optionsFile), ":")
		if err != nil {
			return nil, err
		}
		changed = mergeIn(&f, canonicalValues(shared), fm)
	}
	if in.Sync.Keys && applyKeys(&f, keys, fm) {
		changed = true
	}
	if changed {
		if err := f.Write(se.optionsPath()); err != nil {
			return nil, err
		}
	}
	se.before = canonicalValues(f)
	return se, nil
}

// Finish copies the run's changes out: synced files it changed (newest
// wins) and the option keys it changed. Call it after the run view merged
// back, so it reads the instance's real files.
func (se *Session) Finish() error {
	unlock := se.s.Lock(se.id)
	defer unlock()
	for _, t := range se.files {
		if p := se.path(t); !stamp(p).Equal(se.stamps[t.ID]) {
			if err := newest(p, se.s.SharedPath(t)); err != nil {
				return err
			}
		}
	}
	if !se.options {
		return nil
	}
	f, err := optfile.Read(se.optionsPath(), ":")
	if err != nil {
		return err
	}
	changes := changedSince(se.before, canonicalValues(f))
	if len(changes) == 0 {
		return nil
	}
	sharedPath := filepath.Join(se.s.c.Root, optionsFile)
	shared, err := optfile.Read(sharedPath, ":")
	if err != nil {
		return err
	}
	for _, k := range sortedKeys(changes) {
		shared.Set(k, changes[k])
	}
	return shared.Write(sharedPath)
}

// SessionState is a Session saved beside its run, so a launcher that quit
// while the game ran can still finish it (see Resume).
type SessionState struct {
	ID      string               `json:"id"`
	Files   []string             `json:"files,omitempty"` // target ids
	Options bool                 `json:"options,omitempty"`
	Before  map[string]string    `json:"before,omitempty"`
	Stamps  map[string]time.Time `json:"stamps,omitempty"`
}

// State is what Resume needs to finish the session later.
func (se *Session) State() SessionState {
	st := SessionState{ID: se.id, Options: se.options, Before: se.before, Stamps: se.stamps}
	for _, t := range se.files {
		st.Files = append(st.Files, t.ID)
	}
	return st
}

// Resume rebuilds a saved session for its Finish. Targets turned off since
// are left alone.
func (s *Service) Resume(st SessionState) *Session {
	y := s.settings()
	se := &Session{s: s, id: st.ID, game: s.c.Instances.GameDir(st.ID), before: st.Before, stamps: st.Stamps,
		options: st.Options && s.enabled(y, OptionsID)}
	if se.stamps == nil {
		se.stamps = map[string]time.Time{}
	}
	for _, id := range st.Files {
		if t, ok := find(y, id); ok && s.enabled(y, id) {
			se.files = append(se.files, t)
		}
	}
	return se
}

func (se *Session) path(t Target) string {
	return filepath.Join(se.game, filepath.FromSlash(t.Path))
}

func (se *Session) optionsPath() string { return filepath.Join(se.game, optionsFile) }

// applyKeys writes the shared keybinds: only lines the file has, since keys
// a version (or its mods) lacks would be meaningless, or all of them into a
// fresh file (the game drops what it doesn't know).
func applyKeys(f *optfile.File, keys []settings.SharedKey, fm format) bool {
	changed := false
	for _, sk := range keys {
		line := "key_" + sk.ID
		if f.Exist && !f.Has(line) {
			continue
		}
		k, ok := keycodes.ByName(sk.Key)
		if !ok {
			continue
		}
		v, ok := keycodes.Format(k, fm.legacyKeys)
		if !ok {
			continue
		}
		if old, had := f.Get(line); !had || old != v {
			f.Set(line, v)
			changed = true
		}
	}
	return changed
}

// seedOptions starts the shared options from an instance's, in shared form.
func seedOptions(game, shared string) error {
	f, err := optfile.Read(game, ":")
	if err != nil || !f.Exist {
		return err
	}
	out := optfile.File{Sep: ":"}
	values := f.Values()
	for _, k := range f.Keys() {
		if !ownKeys[k] && validValue(k, values[k]) {
			out.Set(k, canonical(k, values[k]))
		}
	}
	return out.Write(shared)
}

// RunFolder is a shared folder a run assembles (see package runview).
type RunFolder struct {
	Rel    string
	Shared string
}

// RunFolders lists the shared folders an instance's run assembles.
func (s *Service) RunFolders(id string) []RunFolder {
	var out []RunFolder
	for _, t := range s.Folders(id) {
		out = append(out, RunFolder{Rel: t.Path, Shared: s.SharedPath(t)})
	}
	return out
}
