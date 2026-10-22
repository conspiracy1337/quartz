package snapshot

import (
	"context"
	"crypto/sha1" //nolint:gosec // content addressing, as the store does
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"quartz/internal/content"
	"quartz/internal/core/fsx"
	"quartz/internal/modpack"
)

// Take keeps an instance as it is now (content.Snapshotter). Taking an id
// again (a job retried) does nothing.
func (s *Service) Take(ctx context.Context, inst string, r content.SnapshotReq) error {
	unlock := s.lock(inst)
	defer unlock()
	return s.take(ctx, inst, r)
}

// Create takes a snapshot the player asked for.
func (s *Service) Create(ctx context.Context, inst string) (Summary, error) {
	unlock := s.lock(inst)
	defer unlock()
	id := s.newID()
	if err := s.take(ctx, inst, content.SnapshotReq{ID: id, Reason: "manual"}); err != nil {
		return Summary{}, err
	}
	sn, err := read(s.dir(inst), id)
	return s.summary(sn), err
}

// Around takes a snapshot, makes a change and records it in the activity
// feed: for changes made outside a job (the loader update at launch). No
// snapshot means no change.
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

// take writes the snapshot in a temporary folder and renames it into
// place, so a snapshot is there whole or not at all.
func (s *Service) take(ctx context.Context, inst string, r content.SnapshotReq) error {
	if !ValidID(r.ID) {
		return ErrUnknown
	}
	dir := s.dir(inst)
	final := filepath.Join(dir, r.ID)
	if _, err := os.Stat(final); err == nil {
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
	sn := Snapshot{ID: r.ID, At: s.now().UnixMilli(), Title: clip(r.Title, 200), Reason: r.Reason, Scope: r.Scope,
		Version: Version{Minecraft: in.Minecraft, Loader: in.Loader, LoaderVersion: in.LoaderVersion}, State: st, Files: []File{}, Trees: []string{}}
	if in.Pack != nil {
		p := *in.Pack
		sn.Pack = &p
	}
	tmp := final + tmpExt
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, filesDir), 0o700); err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			_ = os.RemoveAll(tmp)
		}
	}()
	if data, err := fsx.ReadCapped(filepath.Join(s.d.Instances.Dir(inst), modpack.ManifestName), maxMeta); err == nil {
		if err := fsx.AtomicWrite(filepath.Join(tmp, modpack.ManifestName), data, fsx.FilePerm); err != nil {
			return err
		}
		sn.Manifest = true
	}
	c, err := s.newCopier(inst, dir, filepath.Join(tmp, filesDir))
	if err != nil {
		return err
	}
	defer c.close()
	for _, tree := range Trees {
		if s.d.Instances.Base(inst, tree) != s.d.Instances.GameDir(inst) {
			sn.State.Shared = append(sn.State.Shared, tree) // shared: other instances own it too
			continue
		}
		c.tree(&sn, tree)
	}
	c.extra(&sn, r.Files)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := write(tmp, sn); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	done = true
	s.emit()
	s.arm()
	return nil
}

// copier copies files from an instance's game folder into a snapshot.
type copier struct {
	game *os.Root
	out  string          // the new snapshot's files folder
	prev map[string]File // the newest earlier snapshot's copies, by path
	from string          // its files folder
}

func (s *Service) newCopier(inst, dir, out string) (*copier, error) {
	game, err := os.OpenRoot(s.d.Instances.GameDir(inst))
	if err != nil {
		return nil, err
	}
	c := &copier{game: game, out: out, prev: map[string]File{}}
	if sn, ok := latest(dir); ok {
		c.from = filepath.Join(dir, sn.ID, filesDir)
		for _, f := range sn.Files {
			if !f.Absent {
				c.prev[f.Rel] = f
			}
		}
	}
	return c, nil
}

func (c *copier) close() { _ = c.game.Close() }

// latest is the newest readable snapshot in a folder.
func latest(dir string) (Snapshot, bool) {
	entries, _ := os.ReadDir(dir)
	for i := len(entries) - 1; i >= 0; i-- { // ReadDir sorts by name: oldest first
		if e := entries[i]; e.IsDir() && ValidID(e.Name()) {
			if sn, err := read(dir, e.Name()); err == nil {
				return sn, true
			}
		}
	}
	return Snapshot{}, false
}

// tree copies every regular file under one folder (links are skipped).
// What doesn't fit the limits is left out and the tree marked partial.
func (c *copier) tree(sn *Snapshot, tree string) {
	count, bytes, partial := 0, int64(0), false
	walkFiles(c.game, tree, func(rel string, info fs.FileInfo) {
		if info.Size() > maxFile || count >= maxTreeFiles || bytes+info.Size() > maxTreeBytes || len(sn.Files) >= maxFiles {
			partial = true
			return
		}
		f, copied, err := c.keep(rel)
		if err != nil {
			partial = true
			return
		}
		sn.Files = append(sn.Files, f)
		sn.Size += copied
		count++
		bytes += f.Size
	})
	sn.Trees = append(sn.Trees, tree)
	if partial {
		sn.Partial = append(sn.Partial, tree)
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
func (c *copier) extra(sn *Snapshot, rels []string) {
	seen := map[string]bool{}
	for _, rel := range rels {
		for _, name := range []string{rel, rel + content.Disabled} {
			if seen[name] || !validFileRel(name) || inTree(name, sn.Trees) || len(sn.Files) >= maxFiles {
				continue
			}
			seen[name] = true
			info, err := c.game.Lstat(name)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				sn.Files = append(sn.Files, File{Rel: name, Absent: true})
			case err == nil && info.Mode().IsRegular() && info.Size() <= maxFile:
				if f, copied, err := c.keep(name); err == nil {
					sn.Files = append(sn.Files, f)
					sn.Size += copied
				}
			}
		}
	}
}

// keep copies one file into the snapshot, or links the earlier snapshot's
// copy when the file hasn't changed since (same size and time).
func (c *copier) keep(rel string) (File, int64, error) {
	dst := filepath.Join(c.out, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return File{}, 0, err
	}
	src, err := c.game.Open(rel)
	if err != nil {
		return File{}, 0, err
	}
	defer func() { _ = src.Close() }()
	info, err := src.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return File{}, 0, errors.New("snapshot: not a regular file")
	}
	if p, ok := c.prev[rel]; ok && p.Size == info.Size() && p.ModNS == info.ModTime().UnixNano() {
		if os.Link(filepath.Join(c.from, filepath.FromSlash(rel)), dst) == nil {
			return p, 0, nil
		}
	}
	n, sum, err := copyHashed(dst, src)
	if err != nil {
		return File{}, 0, err
	}
	return File{Rel: rel, Size: n, ModNS: info.ModTime().UnixNano(), SHA1: sum}, n, nil
}

// copyHashed writes src to a new file at dst and returns its size and sha1.
func copyHashed(dst string, src io.Reader) (int64, string, error) {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fsx.FilePerm) //nolint:gosec // dst is inside the snapshot or a swap's staging name
	if err != nil {
		return 0, "", err
	}
	h := sha1.New() //nolint:gosec // content addressing, as the store does
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(src, maxFile+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxFile {
		err = errors.New("snapshot: the file is too big")
	}
	if err != nil {
		_ = os.Remove(dst)
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
