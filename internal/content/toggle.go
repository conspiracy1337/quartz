package content

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
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
	ErrOneShader    = errors.New("content: only one shader pack can be on at a time")
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

// SetEnabledMany switches several items of one kind on or off at once, then
// rescans once. Shader packs are exclusive: several can only be turned off.
func (s *Service) SetEnabledMany(ctx context.Context, id string, k Kind, items []string, on bool) error {
	switch specs[k].toggle {
	case toggleShader:
		if on {
			return ErrOneShader
		}
		return s.setShader(ctx, id, "")
	case togglePackList:
		v, err := s.view(id)
		if err != nil {
			return err
		}
		packs := v.packs
		for _, item := range items {
			e, ok := v.idx.Entries[item]
			if !ok || e.Kind != k {
				continue
			}
			packs = without(packs, base(e))
			if on {
				packs = append([]string{base(e)}, packs...)
			}
		}
		return s.setPacks(ctx, id, packs)
	}
	var first error
	for _, item := range items {
		e, err := s.entry(id, item)
		if err != nil || e.Kind != k {
			continue
		}
		unlock := s.lock(id)
		if err := s.renameIn(id, e.Rel, toggled(e.Rel, on)); err != nil && first == nil {
			first = err
		}
		unlock()
	}
	if err := s.Scan(ctx, id, k); err != nil {
		return err
	}
	return first
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
	err := s.renameIn(id, e.Rel, toggled(e.Rel, on))
	unlock()
	if err != nil {
		return err
	}
	return s.Scan(ctx, id, e.Kind)
}

// toggled is an item's path when turned on or off by suffix.
func toggled(rel string, on bool) string {
	rel = strings.TrimSuffix(rel, Disabled)
	if !on {
		rel += Disabled
	}
	return rel
}

// renameIn renames inside the item's folder only (that folder may itself be
// a link, which scans follow); a link inside can't lead out.
func (s *Service) renameIn(id, from, to string) error {
	if from == to {
		return nil
	}
	dir := filepath.Join(s.d.Instances.Base(id, from), filepath.FromSlash(path.Dir(from)))
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	from, to = path.Base(from), path.Base(to)
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
	v, err := s.view(id)
	if err != nil {
		return err
	}
	if s.running(id) {
		// Remember the list this edit started from: if the game changes it
		// meanwhile, the player's in-game choice wins (see GameExited).
		base := v.packs
		if v.idx.Pending.Packs != nil {
			base = v.idx.Pending.PacksBase
		} else if opts, err := optfile.Read(v.options, ":"); err == nil {
			base = enabledPacks(opts, v.legacy)
		}
		_, err := ix.Update(func(i *Index) error {
			i.Pending.Packs, i.Pending.PacksBase = append([]string{}, packs...), append([]string{}, base...)
			return nil
		})
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
	l, err := optfile.Read(v.options, ":")
	if err != nil {
		return err
	}
	if err := orderPacks(&l, packs, incompatible, v.formats.WorldVersion, v.legacy); err != nil {
		return err
	}
	if err := l.Write(v.options); err != nil {
		return err
	}
	_, err = ix.Update(func(i *Index) error { i.Pending.Packs, i.Pending.PacksBase = nil, nil; return nil })
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
	v, err := s.view(id)
	if err != nil {
		return err
	}
	mod := s.shaderMod(v).id
	p := s.abs(v.in.ID, shaderConfig(mod))
	if s.running(id) {
		base := v.shader
		if v.idx.Pending.Shader != nil {
			base = v.idx.Pending.ShaderBase
		} else if l, err := optfile.Read(p, "="); err == nil {
			base = activeShader(l, mod)
		}
		_, err := ix.Update(func(i *Index) error { i.Pending.Shader, i.Pending.ShaderBase = &name, base; return nil })
		return err
	}
	l, err := optfile.Read(p, "=")
	if err != nil {
		return err
	}
	setShader(&l, name, mod)
	if err := l.Write(p); err != nil {
		return err
	}
	_, err = ix.Update(func(i *Index) error { i.Pending.Shader, i.Pending.ShaderBase = nil, ""; return nil })
	return err
}

// GameExited applies edits queued while the instance's game ran, unless the
// player changed the same setting in game meanwhile: then theirs wins.
func (s *Service) GameExited(id string) {
	if s.running(id) {
		return // another copy still runs
	}
	ix, err := s.idx.get(id)
	if err != nil {
		return
	}
	unlock := s.lock(id)
	defer unlock()
	v, err := s.view(id)
	if err != nil {
		return
	}
	pend := v.idx.Pending
	if pend.Packs != nil {
		opts, _ := optfile.Read(v.options, ":")
		if now := enabledPacks(opts, v.legacy); !slices.Equal(now, pend.PacksBase) {
			s.d.Log.Info("content: resource packs changed in game; keeping the game's choice")
			_, _ = ix.Update(func(i *Index) error { i.Pending.Packs, i.Pending.PacksBase = nil, nil; return nil })
		} else if err := s.writePacks(id, pend.Packs); err != nil {
			s.d.Log.Warn("content: applying the resource pack order failed", "err", err)
		}
	}
	if pend.Shader != nil {
		mod := s.shaderMod(v).id
		props, _ := optfile.Read(s.abs(id, shaderConfig(mod)), "=")
		if activeShader(props, mod) != pend.ShaderBase {
			s.d.Log.Info("content: shader changed in game; keeping the game's choice")
			_, _ = ix.Update(func(i *Index) error { i.Pending.Shader, i.Pending.ShaderBase = nil, ""; return nil })
		} else if err := s.writeShader(id, *pend.Shader); err != nil {
			s.d.Log.Warn("content: applying the shader choice failed", "err", err)
		}
	}
	s.emit(id, ResourcePacks)
	s.emit(id, Shaders)
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
