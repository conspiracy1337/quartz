package snapshot

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"quartz/internal/content"
	"quartz/internal/modpack"
)

// Take keeps an instance as it is now, before a change (content.Snapshotter):
// a layer in the open session, which it opens if there is none. Taking an id
// again (a job retried) does nothing.
func (s *Service) Take(ctx context.Context, inst string, r content.SnapshotReq) error {
	unlock := s.lock(inst)
	defer unlock()
	return s.take(ctx, inst, r)
}

// Create makes a restore point the player asked for. It stands alone: the
// open session is sealed first, and later changes open a new one.
func (s *Service) Create(ctx context.Context, inst string) (Summary, error) {
	unlock := s.lock(inst)
	defer unlock()
	id := s.newID()
	if err := s.take(ctx, inst, content.SnapshotReq{ID: id, Reason: "manual"}); err != nil {
		return Summary{}, err
	}
	p, newest, err := s.point(inst, id)
	return summary(p, newest), err
}

// Around keeps the instance, makes a change and records it in the activity
// feed: for changes made outside a job (the loader update at launch). No
// layer means no change.
func (s *Service) Around(ctx context.Context, inst, doing, done string, change func() error) error {
	id := s.NewID()
	if id != "" {
		if err := s.Take(ctx, inst, content.SnapshotReq{ID: id, Title: doing, Reason: "loader", Scope: []string{content.ScopeVersion}}); err != nil {
			return err
		}
	}
	if err := change(); err != nil {
		return err
	}
	s.d.Content.Record(inst, content.Activity{Kind: "updated", Title: done, Snapshot: id})
	return nil
}

// take writes the layer, then files it under its point.
func (s *Service) take(ctx context.Context, inst string, r content.SnapshotReq) error {
	if !ValidID(r.ID) {
		return ErrUnknown
	}
	dir := s.dir(inst)
	if _, err := os.Stat(filepath.Join(dir, layersDir, r.ID+jsonExt)); err == nil {
		return nil
	}
	in, err := s.d.Instances.Get(inst)
	if err != nil {
		return err
	}
	st, err := s.d.Content.State(ctx, inst, true)
	if err != nil {
		return err
	}
	list := points(dir)
	manual := r.Reason == "manual"
	p, ok := open(list)
	if manual || !ok {
		p = Point{ID: r.ID, At: s.now().UnixMilli(), Manual: manual, Changes: []Change{}}
	}
	l := Layer{ID: r.ID, Point: p.ID, At: s.now().UnixMilli(), Title: clip(r.Title, 200), Reason: r.Reason, Scope: r.Scope,
		Version: Version{Minecraft: in.Minecraft, Loader: in.Loader, LoaderVersion: in.LoaderVersion}, State: st, Files: []File{}, Trees: []string{}}
	if in.Pack != nil {
		pack := *in.Pack
		l.Pack = &pack
	}
	if f, err := os.Open(filepath.Join(s.d.Instances.Dir(inst), modpack.ManifestName)); err == nil {
		sha, _, added, err := putObject(dir, f)
		_ = f.Close()
		if err != nil {
			return err
		}
		l.Manifest, l.Size = sha, l.Size+added
	}
	c, err := s.newCopier(inst, dir)
	if err != nil {
		return err
	}
	defer c.close()
	for _, tree := range Trees {
		if s.d.Instances.Base(inst, tree) != s.d.Instances.GameDir(inst) {
			l.State.Shared = append(l.State.Shared, tree) // shared: other instances own it too
			continue
		}
		c.tree(&l, tree)
	}
	c.extra(&l, r.Files)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeJSON(dir, layersDir, l.ID, l); err != nil {
		return err
	}
	if manual {
		if cur, ok := open(list); ok {
			s.seal(dir, cur) // its old files are freed by the next purge
		}
		p.Sealed = l.At
	} else {
		p.Changes = append(p.Changes, Change{ID: l.ID, At: l.At, Title: l.Title, Reason: l.Reason, Undoable: true})
		s.trim(dir, &p)
	}
	if err := writeJSON(dir, pointsDir, p.ID, p); err != nil {
		_ = os.Remove(filepath.Join(dir, layersDir, l.ID+jsonExt))
		return err
	}
	s.emit()
	s.arm()
	return nil
}

// trim keeps an open session to maxLayers: the oldest layers after its own
// go, and their changes can then only be rolled back with the whole point.
func (s *Service) trim(dir string, p *Point) {
	n := 1 // the point's own layer
	for _, c := range p.Changes {
		if c.Undoable && c.ID != p.ID {
			n++
		}
	}
	for i := range p.Changes {
		if n <= maxLayers {
			break
		}
		if c := &p.Changes[i]; c.Undoable && c.ID != p.ID {
			_ = os.Remove(filepath.Join(dir, layersDir, c.ID+jsonExt))
			c.Undoable = false
			n--
		}
	}
	if len(p.Changes) > maxChanges {
		p.Changes = p.Changes[len(p.Changes)-maxChanges:]
	}
}

// Seal ends an instance's open session: the game is being launched with it.
// The session keeps only its own layer (the instance as it was last played)
// and the list of its changes.
func (s *Service) Seal(inst string) {
	unlock := s.lock(inst)
	dir := s.dir(inst)
	p, ok := open(points(dir))
	var freed []string
	if ok {
		freed = s.seal(dir, p)
	}
	unlock()
	if ok {
		s.release(freed)
		s.emit()
		s.arm()
	}
}

// seal seals p and drops its other layers, returning the blobs only they
// may have kept; the instance's lock is held.
func (s *Service) seal(dir string, p Point) []string {
	var freed []string
	for i := range p.Changes {
		c := &p.Changes[i]
		if c.Undoable && c.ID != p.ID {
			if l, err := readLayer(dir, c.ID); err == nil {
				for _, it := range l.State.Items {
					freed = append(freed, it.SHA1)
				}
			}
			_ = os.Remove(filepath.Join(dir, layersDir, c.ID+jsonExt))
		}
		c.Undoable = false
	}
	p.Sealed = s.now().UnixMilli()
	if err := writeJSON(dir, pointsDir, p.ID, p); err != nil {
		s.d.Log.Warn("history: sealing a restore point failed", "err", err)
	}
	sweep(dir)
	return freed
}

// copier keeps files from an instance's game folder in the object store.
type copier struct {
	dir  string // the instance's history
	game *os.Root
	prev map[string]File // the newest layer's files, by path
}

func (s *Service) newCopier(inst, dir string) (*copier, error) {
	game, err := os.OpenRoot(s.d.Instances.GameDir(inst))
	if err != nil {
		return nil, err
	}
	c := &copier{dir: dir, game: game, prev: map[string]File{}}
	if all := layers(dir); len(all) > 0 {
		for _, f := range all[0].Files {
			if !f.Absent {
				c.prev[f.Rel] = f
			}
		}
	}
	return c, nil
}

func (c *copier) close() { _ = c.game.Close() }

// tree keeps every regular file under one folder (links are skipped). What
// doesn't fit the limits is left out and the tree marked partial.
func (c *copier) tree(l *Layer, tree string) {
	count, bytes, partial := 0, int64(0), false
	walkFiles(c.game, tree, func(rel string, info fs.FileInfo) {
		if info.Size() > maxFile || count >= maxTreeFiles || bytes+info.Size() > maxTreeBytes || len(l.Files) >= maxFiles {
			partial = true
			return
		}
		f, added, err := c.keep(rel)
		if err != nil {
			partial = true
			return
		}
		l.Files = append(l.Files, f)
		l.Size += added
		count++
		bytes += f.Size
	})
	l.Trees = append(l.Trees, tree)
	if partial {
		l.Partial = append(l.Partial, tree)
	}
}

// walkFiles calls fn with every regular file under tree in the game folder;
// links, unreadable folders (a missing tree too) and swap leftovers are
// skipped.
func walkFiles(game *os.Root, tree string, fn func(rel string, info fs.FileInfo)) {
	_ = fs.WalkDir(game.FS(), tree, func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}
		if d.Type().IsRegular() && validFileRel(rel) {
			if info, err := d.Info(); err == nil {
				fn(rel, info)
			}
		}
		return nil
	})
}

// extra keeps the files a change names outside the trees, on and off, and
// records the ones that aren't there (a rollback deletes them).
func (c *copier) extra(l *Layer, rels []string) {
	seen := map[string]bool{}
	for _, rel := range rels {
		for _, name := range []string{rel, rel + content.Disabled} {
			if seen[name] || !validFileRel(name) || inTree(name, l.Trees) || len(l.Files) >= maxFiles {
				continue
			}
			seen[name] = true
			info, err := c.game.Lstat(name)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				l.Files = append(l.Files, File{Rel: name, Absent: true})
			case err == nil && info.Mode().IsRegular() && info.Size() <= maxFile:
				if f, added, err := c.keep(name); err == nil {
					l.Files = append(l.Files, f)
					l.Size += added
				}
			}
		}
	}
}

// keep puts one file in the object store, unless the newest layer has it
// unchanged (same size and time) and its object is still there. It returns
// the bytes the store grew by.
func (c *copier) keep(rel string) (File, int64, error) {
	src, err := c.game.Open(rel)
	if err != nil {
		return File{}, 0, err
	}
	defer func() { _ = src.Close() }()
	info, err := src.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return File{}, 0, errors.New("snapshot: not a regular file")
	}
	if p, ok := c.prev[rel]; ok && p.Size == info.Size() && p.ModNS == info.ModTime().UnixNano() && hasObject(c.dir, p.SHA1) {
		return p, 0, nil
	}
	sha, n, added, err := putObject(c.dir, src)
	if err != nil {
		return File{}, 0, err
	}
	return File{Rel: rel, Size: n, ModNS: info.ModTime().UnixNano(), SHA1: sha}, added, nil
}
