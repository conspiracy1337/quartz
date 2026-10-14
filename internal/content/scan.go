package content

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"quartz/internal/content/meta"
	"quartz/internal/core/archive"
	"quartz/internal/core/hashx"
)

// Jars bigger than this are listed but never opened for metadata.
const maxParse = 1 << 30

// job is one new or changed item a scan must read.
type job struct {
	id    string
	rel   string
	group string
	dir   bool
	size  int64
	mod   int64
	old   Entry
	had   bool
}

// Scan brings the index of one kind up to date with the folder. Unchanged
// files are only stat'ed; new ones are hashed, adopted into the store and
// parsed once (the catalog remembers every blob). Concurrent calls for the
// same instance and kind share one scan.
func (s *Service) Scan(ctx context.Context, id string, k Kind) error {
	_, err, _ := s.scans.Do(id+"/"+string(k), func() (any, error) { return nil, s.scan(ctx, id, k) })
	return err
}

func (s *Service) scan(ctx context.Context, id string, k Kind) error {
	// A missing folder must not empty the index: the drive may come back.
	if err := s.d.Instances.CheckGameDir(id); err != nil {
		return err
	}
	unlock := s.lock(id)
	defer unlock()
	ix, err := s.idx.get(id)
	if err != nil {
		return err
	}
	game := s.base(id, k)
	sp := specs[k]
	cur := ix.Get()
	s.purgeRemovals(id, cur)
	next := map[string]Entry{}
	var jobs []job
	if root, err := os.OpenRoot(game); err == nil {
		jobs = s.walk(root, k, sp, cur, next)
		_ = root.Close()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, e := range s.read(ctx, game, k, sp, jobs, !s.inPlace(id)) {
		next[itemID(k, e.Rel)] = e
	}
	changed := false
	_, err = ix.Update(func(i *Index) error {
		for eid, e := range i.Entries {
			if e.Kind == k {
				if _, ok := next[eid]; !ok {
					delete(i.Entries, eid)
					delete(i.Updates, eid)
					changed = true
				}
			}
		}
		for eid, e := range next {
			if old, ok := i.Entries[eid]; !ok || !sameEntry(old, e) {
				changed = true
			}
			i.Entries[eid] = e
		}
		if k == Mods {
			miss := s.missing(id, *i)
			if !sameMissing(i.Missing, miss) {
				i.Missing, changed = miss, true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if changed {
		s.emit(id, k)
	}
	s.identifyLater(id, k)
	return ctx.Err()
}

// walk lists every item; unchanged ones go straight into next, the rest
// become jobs.
func (s *Service) walk(root *os.Root, k Kind, sp spec, cur Index, next map[string]Entry) []job {
	var jobs []job
	for _, f := range sp.folders(root) {
		dir, err := root.Open(f.Rel)
		if err != nil {
			continue
		}
		entries, _ := dir.ReadDir(-1)
		_ = dir.Close()
		for _, de := range entries {
			info, err := de.Info()
			if err != nil {
				continue
			}
			ok, _ := sp.accepts(de.Name(), info.Mode())
			if !ok || partial(de.Name()) {
				continue
			}
			rel := path.Join(f.Rel, de.Name())
			eid := itemID(k, rel)
			if _, dup := next[eid]; dup {
				continue // both x.jar and x.jar.disabled: the enabled one wins
			}
			j := job{id: eid, rel: rel, group: f.Group, dir: info.IsDir(), size: info.Size(), mod: info.ModTime().UnixNano()}
			if j.dir {
				j.size = 0
			}
			j.old, j.had = cur.Entries[eid]
			// Same size and time means the same file, even renamed by a toggle.
			if j.had && j.old.Size == j.size && j.old.ModNS == j.mod && (j.dir == j.old.Dir) && (j.dir || j.old.SHA1 != "") {
				if _, known := s.cat.get(j.old.SHA1); j.dir || known {
					e := j.old
					e.Rel, e.Group = rel, f.Group
					next[eid] = e
					continue
				}
			}
			jobs = append(jobs, j)
		}
	}
	return jobs
}

// partial names files still being written by a browser or the launcher.
func partial(name string) bool {
	for _, ext := range []string{".part", ".crdownload", ".tmp", ".download"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			return true
		}
	}
	return false
}

// read processes jobs on a few workers; a file that fails is left out.
func (s *Service) read(ctx context.Context, game string, k Kind, sp spec, jobs []job, adopt bool) []Entry {
	if len(jobs) == 0 {
		return nil
	}
	out := make([]Entry, 0, len(jobs))
	var mu sync.Mutex
	work := make(chan job)
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), 8, len(jobs)) {
		wg.Go(func() {
			for j := range work {
				e, err := s.readOne(game, k, sp, j, adopt)
				if err != nil {
					s.d.Log.Debug("content: skipping an item", "item", j.rel, "err", err)
					continue
				}
				mu.Lock()
				out = append(out, e)
				mu.Unlock()
			}
		})
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		work <- j
	}
	close(work)
	wg.Wait()
	return out
}

func (s *Service) readOne(game string, k Kind, sp spec, j job, adopt bool) (Entry, error) {
	e := Entry{Kind: k, Rel: j.rel, Group: j.group, Dir: j.dir, Size: j.size, ModNS: j.mod, Added: s.now().Unix()}
	if j.had {
		e.Added = j.old.Added
	}
	abs := filepath.Join(game, filepath.FromSlash(j.rel))
	if j.dir {
		sub, err := os.OpenRoot(abs)
		if err != nil {
			return Entry{}, err
		}
		defer func() { _ = sub.Close() }()
		var src meta.Source = meta.Dir{Root: sub}
		if strings.HasSuffix(j.rel, Disabled) {
			// Turned off by name only (by hand, or an older launcher): the
			// game would still load it until its manifest is renamed too.
			if _, err := sub.Lstat(packManifest); err == nil {
				_ = sub.Rename(packManifest, offManifest)
			}
			src = offPack{meta.Dir{Root: sub}}
		}
		info, err := sp.parse(src, nil, s.d.Icons)
		if err != nil {
			return Entry{}, err
		}
		e.Info = &info
		return e, nil
	}
	h, err := hashStable(abs, j)
	if err != nil {
		return Entry{}, err
	}
	e.SHA1 = h.SHA1
	s.ensureKnown(abs, h.SHA1, sp)
	if !adopt {
		return e, nil
	}
	if _, err := s.d.Store.Adopt(abs, h); err != nil {
		s.d.Log.Warn("content: could not adopt a file into the store", "item", j.rel, "err", err)
	}
	// Adopting may swap the file for a link to the blob: record what is there now.
	if info, err := os.Lstat(abs); err == nil {
		e.Size, e.ModNS = info.Size(), info.ModTime().UnixNano()
	}
	return e, nil
}

// inPlace reports whether an instance plays in another launcher's folder,
// whose files are left exactly as they are (no adopting, no shared links).
func (s *Service) inPlace(id string) bool {
	in, err := s.d.Instances.Get(id)
	return err == nil && in.InPlace
}

// place puts a stored file into an instance: a shared placement, or a
// private copy in another launcher's folder.
func (s *Service) place(id, sha1, abs string) error {
	var err error
	if s.inPlace(id) {
		_, err = s.d.Store.PlacePrivate(sha1, abs)
	} else {
		_, err = s.d.Store.Place(sha1, abs)
	}
	return err
}

// hashStable hashes a file and makes sure it didn't change meanwhile (a
// download still being written is picked up by the next scan instead).
func hashStable(abs string, j job) (hashx.Multi, error) {
	f, err := os.Open(abs) //nolint:gosec // listed through the instance's os.Root a moment ago
	if err != nil {
		return hashx.Multi{}, err
	}
	defer func() { _ = f.Close() }()
	h, err := hashx.MultiOf(f)
	if err != nil {
		return hashx.Multi{}, err
	}
	info, err := f.Stat()
	if err != nil {
		return hashx.Multi{}, err
	}
	if info.Size() != j.size || info.ModTime().UnixNano() != j.mod || h.Size != j.size {
		return hashx.Multi{}, errors.New("content: the file is still changing")
	}
	return h, nil
}

// ensureKnown parses a blob the catalog hasn't seen.
func (s *Service) ensureKnown(abs, sha1 string, sp spec) { s.current(abs, sha1, sp) }

// current is a blob's catalog entry, parsed (again) when the catalog hasn't
// seen it or an older parser read it. abs is a copy of the blob on disk.
func (s *Service) current(abs, sha1 string, sp spec) Known {
	if k, ok := s.cat.get(sha1); ok && (k.Info != nil || k.Bad) && k.Rev >= metaRev {
		return k
	}
	if _, err := os.Stat(abs); err != nil {
		k, _ := s.cat.get(sha1)
		return k // nothing to read now; keep what is known
	}
	info, err := parseFile(abs, sp, s.d.Icons)
	s.cat.update(sha1, func(k *Known) {
		if err != nil {
			k.Info, k.Bad = nil, true
		} else {
			k.Info, k.Bad = &info, false
		}
		k.Rev = metaRev
	})
	k, _ := s.cat.get(sha1)
	return k
}

func parseFile(abs string, sp spec, ic meta.Icons) (meta.Info, error) {
	if info, err := os.Stat(abs); err != nil || info.Size() > maxParse {
		return meta.Info{}, meta.ErrNotContent
	}
	zr, err := archive.Open(abs)
	if err != nil {
		return meta.Info{}, err
	}
	defer func() { _ = zr.Close() }()
	return sp.parse(zr, zr, ic)
}

func sameEntry(a, b Entry) bool {
	return a.Rel == b.Rel && a.Size == b.Size && a.ModNS == b.ModNS && a.SHA1 == b.SHA1 && a.Group == b.Group
}

func sameMissing(a, b *Missing) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// entriesOf returns one kind's entries, by id.
func entriesOf(i Index, k Kind) map[string]Entry {
	out := map[string]Entry{}
	for id, e := range maps.All(i.Entries) {
		if e.Kind == k {
			out[id] = e
		}
	}
	return out
}
