package content

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"time"

	"quartz/internal/content/packwiz"
	"quartz/internal/modplatform"
)

// A file no platform knew is asked about again after this long.
const reidentify = 24 * time.Hour

// identifyLater identifies new files of one kind in the background; the
// rows update through content:changed when it finds something.
func (s *Service) identifyLater(id string, k Kind) {
	if len(s.d.Platforms) == 0 {
		return
	}
	key := id + "/" + string(k)
	if _, busy := s.identifying.LoadOrStore(key, struct{}{}); busy {
		return
	}
	go func() {
		defer s.identifying.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if n, err := s.Identify(ctx, id, k); err != nil {
			s.d.Log.Info("content: identification incomplete", "instance", id, "kind", k, "err", err)
		} else if n > 0 {
			s.emit(id, k)
		}
	}()
}

// Identify asks the platforms about every file of one kind not identified
// yet, records what they know, and writes .pw.toml files for newly found
// items (as Prism does). It returns how many items it newly identified.
func (s *Service) Identify(ctx context.Context, id string, k Kind) (int, error) {
	if len(s.d.Platforms) == 0 {
		return 0, nil
	}
	ix, err := s.idx.get(id)
	if err != nil {
		return 0, err
	}
	now := s.now()
	var ask []modplatform.Hashes
	seen := map[string]bool{}
	for _, e := range entriesOf(ix.Get(), k) {
		if e.SHA1 == "" || seen[e.SHA1] {
			continue
		}
		kn, _ := s.cat.get(e.SHA1)
		if kn.Ident() != nil || (kn.IdentAt > 0 && now.Sub(time.Unix(kn.IdentAt, 0)) < reidentify) {
			continue
		}
		b, ok := s.d.Store.Blob(e.SHA1)
		if !ok {
			continue
		}
		seen[e.SHA1] = true
		ask = append(ask, modplatform.Hashes{SHA1: e.SHA1, SHA512: b.SHA512, Murmur2: b.Murmur2})
	}
	if len(ask) == 0 {
		return 0, nil
	}
	found, err := s.lookup(ctx, ask)
	if err != nil && len(found) == 0 {
		return 0, err
	}
	n := 0
	for _, l := range ask {
		f, ok := found[l.SHA1]
		s.cat.update(l.SHA1, func(kn *Known) {
			if ok {
				kn.Modrinth, kn.CurseForge = f.modrinth, f.curseforge
			}
			if err == nil || ok {
				kn.IdentAt = now.Unix()
			}
		})
		if ok && (f.modrinth != nil || f.curseforge != nil) {
			n++
		}
	}
	if n > 0 {
		s.writeMetadata(id, k)
	}
	return n, err
}

// writeMetadata adds a .pw.toml for identified items that have none.
func (s *Service) writeMetadata(id string, k Kind) {
	unlock := s.lock(id)
	defer unlock()
	v, err := s.view(id)
	if err != nil {
		return
	}
	for _, e := range entriesOf(v.idx, k) {
		kn := v.known[e.SHA1]
		ident := kn.Ident()
		if ident == nil {
			continue
		}
		if _, ok := s.pwFor(&v, e); ok {
			continue
		}
		m := metaFor(e, kn, ident)
		if b, ok := s.d.Store.Blob(e.SHA1); ok {
			m.Download.HashFormat, m.Download.Hash = "sha512", b.SHA512
		}
		dir := filepath.Join(v.game, filepath.FromSlash(path.Join(path.Dir(e.Rel), specs[k].indexIn)))
		if err := packwiz.Write(dir, m); err != nil {
			s.d.Log.Warn("content: writing metadata failed", "item", e.Rel, "err", err)
		}
	}
}

// found is what each platform knows about one file.
type found struct{ modrinth, curseforge *Ident }

// lookup asks every ready platform about the files, then fetches the
// projects of what matched for names, slugs and authors.
func (s *Service) lookup(ctx context.Context, files []modplatform.Hashes) (map[string]found, error) {
	out := map[string]found{}
	var errs []error
	for _, p := range s.d.Platforms {
		if !p.Ready() {
			continue
		}
		vs, err := p.Identify(ctx, files)
		if err != nil {
			errs = append(errs, err)
		}
		var ids []string
		for _, v := range vs {
			ids = append(ids, v.ProjectID)
		}
		projects, err := p.Projects(ctx, ids)
		if err != nil {
			errs = append(errs, err)
		}
		for sha1, v := range vs {
			f := out[sha1]
			id := identOf(v, projects[v.ProjectID])
			if p.Source() == modplatform.CurseForge {
				f.curseforge = id
			} else {
				f.modrinth = id
			}
			out[sha1] = f
		}
	}
	return out, errors.Join(errs...)
}
