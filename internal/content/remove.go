package content

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Removed items wait here, inside the game folder (so moving them is a
// rename even for a game folder on another drive), until Undo or purge.
const (
	heldDir  = ".quartz/removed"
	holdTime = 15 * time.Minute
)

// ErrUndoExpired means the removed items are already gone for good.
var ErrUndoExpired = errors.New("content: these items can't be brought back any more")

// Remove moves items aside and returns a token for Undo. They are deleted
// for good after a while; the store keeps their blobs a week longer.
func (s *Service) Remove(ctx context.Context, id string, k Kind, items []string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b[:])
	unlock := s.lock(id)
	names, err := s.remove(id, k, items, token)
	unlock()
	if err != nil {
		return "", err
	}
	s.record(id, Activity{Kind: "removed", Title: removedTitle(k, names), Detail: strings.Join(names, ", ")})
	return token, s.Scan(ctx, id, k)
}

func (s *Service) remove(id string, k Kind, items []string, token string) ([]string, error) {
	ix, err := s.idx.get(id)
	if err != nil {
		return nil, err
	}
	v, err := s.view(id)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(v.game)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	held := path.Join(heldDir, token)
	if err := root.MkdirAll(held, 0o700); err != nil {
		return nil, err
	}
	rem := removal{At: s.now().Unix()}
	var names []string
	for n, item := range items {
		e, ok := v.idx.Entries[item]
		if !ok || e.Kind != k {
			continue
		}
		r := removedEntry{ID: item, Entry: e, Held: path.Join(held, strconv.Itoa(n)+"-"+path.Base(e.Rel))}
		if err := root.Rename(e.Rel, r.Held); err != nil {
			if s.running(id) {
				err = ErrGameHasFiles
			}
			_ = s.restore(root, rem.Items) // put back what moved before the failure
			return nil, err
		}
		if pw, ok := s.pwFor(&v, e); ok {
			r.PWRel = path.Join(path.Dir(e.Rel), specs[k].indexIn, pw.Slug+".pw.toml")
			r.PW = path.Join(held, strconv.Itoa(n)+".pw.toml")
			if root.Rename(r.PWRel, r.PW) != nil {
				r.PW, r.PWRel = "", ""
			}
		}
		rem.Items = append(rem.Items, r)
		names = append(names, displayName(e, v.known[e.SHA1]))
	}
	if len(rem.Items) == 0 {
		_ = root.Remove(held)
		return nil, ErrNoItem
	}
	_, err = ix.Update(func(i *Index) error { i.Removed[token] = rem; return nil })
	return names, err
}

// restore moves held items back (Undo, or a failed removal).
func (s *Service) restore(root *os.Root, items []removedEntry) error {
	var errs []error
	for _, r := range items {
		if _, err := root.Lstat(r.Entry.Rel); err == nil {
			errs = append(errs, ErrNameTaken)
			continue
		}
		errs = append(errs, root.Rename(r.Held, r.Entry.Rel))
		if r.PW != "" {
			_ = root.MkdirAll(path.Dir(r.PWRel), 0o700)
			_ = root.Rename(r.PW, r.PWRel)
		}
	}
	return errors.Join(errs...)
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
	root, err := os.OpenRoot(s.d.Instances.GameDir(id))
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	err = s.restore(root, rem.Items)
	_ = root.Remove(path.Join(heldDir, token))
	_, _ = ix.Update(func(i *Index) error { delete(i.Removed, token); return nil })
	return rem.Items[0].Entry.Kind, err
}

// purgeRemovals deletes held items whose undo time is over. Store files are
// unlinked through the store, which keeps the blob sealed.
func (s *Service) purgeRemovals(id, game string, cur Index) {
	cutoff := s.now().Add(-holdTime).Unix()
	var done []string
	for token, rem := range cur.Removed {
		if rem.At > cutoff {
			continue
		}
		for _, r := range rem.Items {
			abs := filepath.Join(game, filepath.FromSlash(r.Held))
			if r.Entry.Dir {
				_ = os.RemoveAll(abs)
			} else {
				_ = s.d.Store.Unlink(abs, r.Entry.SHA1)
			}
		}
		_ = os.RemoveAll(filepath.Join(game, filepath.FromSlash(path.Join(heldDir, token))))
		done = append(done, token)
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
