package content

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"quartz/internal/holdbin"
)

// ErrUndoExpired means the removed items are already gone for good.
var ErrUndoExpired = errors.New("content: these items can't be brought back any more")

// Remove moves items aside and returns a token for Undo. They are deleted
// for good after a while; the store keeps their blobs a week longer.
func (s *Service) Remove(ctx context.Context, id string, k Kind, items []string) (string, error) {
	unlock := s.lock(id)
	token, names, err := s.remove(id, k, items)
	unlock()
	if err != nil {
		return "", err
	}
	s.record(id, Activity{Kind: "removed", Title: removedTitle(k, names), Detail: strings.Join(names, ", ")})
	return token, s.Scan(ctx, id, k)
}

func (s *Service) remove(id string, k Kind, items []string) (string, []string, error) {
	ix, err := s.idx.get(id)
	if err != nil {
		return "", nil, err
	}
	v, err := s.view(id)
	if err != nil {
		return "", nil, err
	}
	root, err := os.OpenRoot(s.base(id, k))
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = root.Close() }()
	var moves []holdbin.Move
	var picked []removedEntry
	var names []string
	for _, item := range items {
		e, ok := v.idx.Entries[item]
		if !ok || e.Kind != k {
			continue
		}
		moves = append(moves, holdbin.Move{Rel: e.Rel})
		if pw, ok := s.pwFor(&v, e); ok {
			moves = append(moves, holdbin.Move{Rel: path.Join(path.Dir(e.Rel), specs[k].indexIn, pw.Slug+".pw.toml"), Optional: true})
		}
		picked = append(picked, removedEntry{ID: item, Entry: e})
		names = append(names, displayName(e, v.known[e.SHA1]))
	}
	if len(picked) == 0 {
		return "", nil, ErrNoItem
	}
	token, held, err := holdbin.Hold(root, moves, s.now())
	if err != nil {
		if s.running(id) {
			err = ErrGameHasFiles
		}
		return "", nil, err
	}
	for i := range picked {
		for _, h := range held {
			if h.Rel == picked[i].Entry.Rel {
				picked[i].Held = h.Held
			}
		}
	}
	rem := removal{At: s.now().Unix(), Items: picked}
	_, err = ix.Update(func(i *Index) error { i.Removed[token] = rem; return nil })
	return token, names, err
}

// Undo brings back what Remove moved aside.
func (s *Service) Undo(ctx context.Context, id, token string) error {
	unlock := s.lock(id)
	k, err := s.undo(id, token)
	unlock()
	if err != nil {
		return err
	}
	return s.Scan(ctx, id, k)
}

func (s *Service) undo(id, token string) (Kind, error) {
	ix, err := s.idx.get(id)
	if err != nil {
		return "", err
	}
	rem, ok := ix.Get().Removed[token]
	if !ok || len(rem.Items) == 0 {
		return "", ErrUndoExpired
	}
	root, err := os.OpenRoot(s.base(id, rem.Items[0].Entry.Kind))
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	_, err = holdbin.Restore(root, token)
	switch {
	case errors.Is(err, holdbin.ErrExpired):
		err = ErrUndoExpired
	case errors.Is(err, holdbin.ErrNameTaken):
		err = ErrNameTaken
	}
	_, _ = ix.Update(func(i *Index) error { delete(i.Removed, token); return nil })
	return rem.Items[0].Entry.Kind, err
}

// purgeRemovals deletes held items whose undo time is over. Store files are
// unlinked through the store first, which keeps the blob sealed.
func (s *Service) purgeRemovals(id string, cur Index) {
	now := s.now()
	cutoff := now.Add(-holdbin.Keep).Unix()
	var done []string
	for token, rem := range cur.Removed {
		if rem.At > cutoff {
			continue
		}
		for _, r := range rem.Items {
			if !r.Entry.Dir && r.Held != "" {
				_ = s.d.Store.Unlink(filepath.Join(s.base(id, r.Entry.Kind), filepath.FromSlash(r.Held)), r.Entry.SHA1)
			}
		}
		done = append(done, token)
	}
	// Held items wait in the folder they came from: the game folder, or the
	// shared one for shared kinds.
	seen := map[string]bool{}
	for _, k := range Kinds {
		if b := s.base(id, k); !seen[b] {
			seen[b] = true
			if root, err := os.OpenRoot(b); err == nil {
				holdbin.Purge(root, now, nil)
				_ = root.Close()
			}
		}
	}
	if len(done) == 0 {
		return
	}
	if ix, err := s.idx.get(id); err == nil {
		_, _ = ix.Update(func(i *Index) error {
			for _, t := range done {
				delete(i.Removed, t)
			}
			return nil
		})
	}
}

func removedTitle(k Kind, names []string) string {
	if len(names) == 1 {
		return "Removed " + names[0]
	}
	return "Removed " + strconv.Itoa(len(names)) + " " + noun(k, len(names))
}

// noun names a kind for counts: "mod", "resource packs", …
func noun(k Kind, n int) string {
	one := map[Kind]string{Mods: "mod", ResourcePacks: "resource pack", Shaders: "shader pack", DataPacks: "data pack"}[k]
	if n == 1 {
		return one
	}
	return one + "s"
}
