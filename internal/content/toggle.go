package content

import (
	"context"
	"errors"
	"os"
	"path"
	"strconv"
	"strings"

	"quartz/internal/content/packwiz"
	"quartz/internal/core/optfile"
)

// Errors the UI shows as they are.
var (
	ErrNoItem       = errors.New("content: that item is gone; the list was refreshed")
	ErrNameTaken    = errors.New("content: a file with that name is already there")
	ErrNotLockable  = errors.New("content: only items found on Modrinth or CurseForge can be locked")
	ErrGameHasFiles = errors.New("content: close the game first; it is using this file")
)

// entry looks an item up in the current index.
func (s *Service) entry(id, item string) (Entry, error) {
	ix, err := s.idx.get(id)
	if err != nil {
		return Entry{}, err
	}
	e, ok := ix.Get().Entries[item]
	if !ok {
		return Entry{}, ErrNoItem
	}
	return e, nil
}

// SetEnabled switches one item on or off, the way its kind does it.
func (s *Service) SetEnabled(ctx context.Context, id string, k Kind, item string, on bool) error {
	e, err := s.entry(id, item)
	if err != nil || e.Kind != k {
		return ErrNoItem
	}
	switch specs[k].toggle {
	case togglePackList:
		v, err := s.view(id)
		if err != nil {
			return err
		}
		packs := without(v.packs, base(e))
		if on {
			packs = append([]string{base(e)}, packs...)
		}
		return s.setPacks(ctx, id, packs)
	case toggleShader:
		name := ""
		if on {
			name = base(e)
		}
		return s.setShader(ctx, id, name)
	}
	return s.rename(ctx, id, e, on)
}

// SetActiveShader makes one shader pack the active one ("" turns shaders off).
func (s *Service) SetActiveShader(ctx context.Context, id, item string) error {
	if item == "" {
		return s.setShader(ctx, id, "")
	}
	return s.SetEnabled(ctx, id, Shaders, item, true)
}

// SetPackOrder sets which resource packs are on and their order, top first.
func (s *Service) SetPackOrder(ctx context.Context, id string, items []string) error {
	ix, err := s.idx.get(id)
	if err != nil {
		return err
	}
	cur := ix.Get()
	names := make([]string, 0, len(items))
	for _, item := range items {
		e, ok := cur.Entries[item]
		if !ok || e.Kind != ResourcePacks {
			return ErrNoItem
		}
		names = append(names, base(e))
	}
	return s.setPacks(ctx, id, names)
}

func without(list []string, s string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

// rename adds or removes the .disabled suffix.
func (s *Service) rename(ctx context.Context, id string, e Entry, on bool) error {
	unlock := s.lock(id)
	from := e.Rel
	to := strings.TrimSuffix(from, Disabled)
	if !on {
		to += Disabled
	}
	err := s.renameIn(id, from, to)
	unlock()
	if err != nil {
		return err
	}
	return s.Scan(ctx, id, e.Kind)
}

// renameIn renames inside the item's folder only; a link can't lead out.
func (s *Service) renameIn(id, from, to string) error {
	if from == to {
		return nil
	}
	root, err := os.OpenRoot(s.d.Instances.Base(id, from))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if _, err := root.Lstat(to); err == nil {
		return ErrNameTaken
	}
	if err := root.Rename(from, to); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNoItem
		}
		if s.running(id) {
			return ErrGameHasFiles
		}
		return err
	}
	if info, err := root.Lstat(to); err == nil && info.IsDir() {
		// A folder pack: its manifest decides whether the game loads it.
		on, off := path.Join(to, packManifest), path.Join(to, offManifest)
		if strings.HasSuffix(to, Disabled) {
			_ = root.Rename(on, off)
		} else {
			_ = root.Rename(off, on)
		}
	}
	return nil
}

// setPacks writes the resource pack list, or queues it while the game runs.
func (s *Service) setPacks(ctx context.Context, id string, packs []string) error {
	unlock := s.lock(id)
	err := s.writePacks(id, packs)
	unlock()
	if err != nil {
		return err
	}
	s.emit(id, ResourcePacks)
	return ctx.Err()
}

func (s *Service) writePacks(id string, packs []string) error {
	ix, err := s.idx.get(id)
	if err != nil {
		return err
	}
	if s.running(id) {
		_, err := ix.Update(func(i *Index) error { i.Pending.Packs = append([]string{}, packs...); return nil })
		return err
	}
	v, err := s.view(id)
	if err != nil {
		return err
	}
	incompatible := map[string]bool{}
	for _, e := range entriesOf(v.idx, ResourcePacks) {
		info := e.Info
		if k := v.known[e.SHA1]; k.Info != nil {
			info = k.Info
		}
		if info != nil && s.formatLabel(&v, ResourcePacks, info) != "" {
			incompatible[base(e)] = true
		}
	}
	l, err := optfile.Read(optionsPath(v.game), ":")
	if err != nil {
		return err
	}
	orderPacks(&l, packs, incompatible, v.formats.WorldVersion)
	if err := l.Write(optionsPath(v.game)); err != nil {
		return err
	}
	_, err = ix.Update(func(i *Index) error { i.Pending.Packs = nil; return nil })
	return err
}

// setShader selects the shader pack, or queues it while the game runs.
func (s *Service) setShader(ctx context.Context, id, name string) error {
	unlock := s.lock(id)
	err := s.writeShader(id, name)
	unlock()
	if err != nil {
		return err
	}
	s.emit(id, Shaders)
	return ctx.Err()
}

func (s *Service) writeShader(id, name string) error {
	ix, err := s.idx.get(id)
	if err != nil {
		return err
	}
	if s.running(id) {
		_, err := ix.Update(func(i *Index) error { i.Pending.Shader = &name; return nil })
		return err
	}
	v, err := s.view(id)
	if err != nil {
		return err
	}
	p := s.abs(v.in.ID, shaderConfig(s.oculus(v)))
	l, err := optfile.Read(p, "=")
	if err != nil {
		return err
	}
	setShader(&l, name)
	if err := l.Write(p); err != nil {
		return err
	}
	_, err = ix.Update(func(i *Index) error { i.Pending.Shader = nil; return nil })
	return err
}

// GameExited applies edits queued while the instance's game ran.
func (s *Service) GameExited(id string) {
	if s.running(id) {
		return // another copy still runs
	}
	ix, err := s.idx.get(id)
	if err != nil {
		return
	}
	pend := ix.Get().Pending
	if pend.Packs != nil {
		unlock := s.lock(id)
		err := s.writePacks(id, pend.Packs)
		unlock()
		if err != nil {
			s.d.Log.Warn("content: applying the resource pack order failed", "err", err)
		}
	}
	if pend.Shader != nil {
		unlock := s.lock(id)
		err := s.writeShader(id, *pend.Shader)
		unlock()
		if err != nil {
			s.d.Log.Warn("content: applying the shader choice failed", "err", err)
		}
	}
}

// SetLocked locks an item's version (update checks skip it) or unlocks it.
// The lock lives in the item's .pw.toml, as Prism and packwiz keep it.
func (s *Service) SetLocked(id, item string, locked bool) error {
	e, err := s.entry(id, item)
	if err != nil {
		return err
	}
	unlock := s.lock(id)
	defer unlock()
	v, err := s.view(id)
	if err != nil {
		return err
	}
	dir := s.abs(v.in.ID, path.Join(path.Dir(e.Rel), specs[e.Kind].indexIn))
	pw, ok := s.pwFor(&v, e)
	if !ok {
		k := v.known[e.SHA1]
		ident := k.Ident()
		if ident == nil {
			return ErrNotLockable
		}
		pw = metaFor(e, k, ident)
	}
	pw.Pin = locked
	if err := packwiz.Write(dir, pw); err != nil {
		return err
	}
	s.emit(id, e.Kind)
	return nil
}

// metaFor builds a .pw.toml for an identified item.
func metaFor(e Entry, k Known, ident *Ident) packwiz.Meta {
	m := packwiz.Meta{Slug: packwiz.Slug(orElse(ident.Slug, displayName(e, k))), Name: displayName(e, k), Filename: base(e),
		Side: "both", Loaders: ident.Loaders, MCVersions: ident.Game, ReleaseType: ident.ReleaseType, VersionNumber: ident.Version}
	if k.Modrinth != nil {
		m.Modrinth = &packwiz.Modrinth{ModID: k.Modrinth.ProjectID, Version: k.Modrinth.VersionID}
	}
	if k.CurseForge != nil {
		pid, _ := strconv.ParseInt(k.CurseForge.ProjectID, 10, 64)
		fid, _ := strconv.ParseInt(k.CurseForge.VersionID, 10, 64)
		m.CurseForge = &packwiz.CurseForge{ProjectID: pid, FileID: fid}
	}
	m.Download = packwiz.Download{Mode: "url", URL: ident.URL}
	if ident.Source == "curseforge" {
		m.Download.Mode = "metadata:curseforge"
	}
	return m
}
