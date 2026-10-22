package content

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"quartz/internal/content/packwiz"
	"quartz/internal/instance"
	"quartz/internal/modplatform"
)

// Every change that replaces files (updates, version changes, modpack
// updates) asks a Snapshotter (package snapshot) to keep the instance as it
// was first, so the change can be rolled back.

// SnapshotReq asks for a snapshot before a change.
type SnapshotReq struct {
	ID     string   // from Snapshotter.NewID
	Title  string   // the change, as a gerund: "updating 12 mods"
	Reason string   // update | install | version | loader | modpack | manual | rollback
	Scope  []string // what the change touches (Scope* keys); nil is everything
	// Files are instance-relative files outside the captured folders whose
	// old copies to keep (a modpack's overrides).
	Files []string
}

// Snapshotter keeps snapshots of instances.
type Snapshotter interface {
	// NewID names a snapshot to take; "" when snapshots are turned off.
	NewID() string
	// Take keeps the instance as it is now. Taking an id again does nothing.
	Take(ctx context.Context, instance string, r SnapshotReq) error
}

// Started is a change running as a job, and the snapshot it takes first
// ("" when snapshots are off).
type Started struct {
	Job      string `json:"job"`
	Snapshot string `json:"snapshot,omitempty"`
}

// ErrSnapshot means the snapshot before a change failed, so nothing changed.
var ErrSnapshot = errors.New("content: couldn't save a snapshot first, so nothing was changed. Snapshots can be turned off in Settings → Storage")

// Scope keys name what a change touches; a rollback offers exactly that.
const (
	ScopeVersion = "version" // Minecraft and the loader
	ScopePack    = "pack"    // the modpack's version and manifest
	ScopeFiles   = "files"   // config and other files
	ScopePacks   = "packs"   // the resource pack list
	ScopeShader  = "shader"  // the active shader pack
)

// ScopeRel names an item by its path, on or off.
func ScopeRel(rel string) string { return "rel:" + strings.TrimSuffix(rel, Disabled) }

// ScopeProject names an item by its project on a platform.
func ScopeProject(src, project string) string { return "id:" + src + ":" + project }

// snapshot starts a snapshot request for a change; nil when snapshots are off.
func (s *Service) snapshot(title, reason string, scope []string) *SnapshotReq {
	if s.d.Snapshots == nil {
		return nil
	}
	id := s.d.Snapshots.NewID()
	if id == "" {
		return nil
	}
	return &SnapshotReq{ID: id, Title: title, Reason: reason, Scope: scope}
}

// take keeps the snapshot r asks for (none when r is nil).
func (s *Service) take(ctx context.Context, inst string, r *SnapshotReq) error {
	if r == nil {
		return nil
	}
	if err := s.d.Snapshots.Take(ctx, inst, *r); err != nil {
		s.d.Log.Warn("content: snapshot failed", "instance", inst, "err", err)
		return ErrSnapshot
	}
	return nil
}

// ID is the request's snapshot id ("" for none).
func (r *SnapshotReq) id() string {
	if r == nil {
		return ""
	}
	return r.ID
}

// scopeOf names the items planned changes touch: the item each replaces and
// the project each puts in.
func scopeOf(picked []*planned, extra ...string) []string {
	out := append([]string{}, extra...)
	for _, pi := range picked {
		if pi.item != "" {
			out = append(out, ScopeRel(pi.old.Rel))
		}
		out = append(out, ScopeProject(string(pi.v.Source), pi.v.ProjectID))
	}
	return out
}

// StateItem is one item as a snapshot keeps it.
type StateItem struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	Rel  string `json:"rel"` // with .disabled when off
	SHA1 string `json:"sha1"`
	// Keys name the item apart from its path: its projects and mod id.
	Keys    []string      `json:"keys,omitempty"`
	Name    string        `json:"name"`
	Version string        `json:"version,omitempty"`
	Icon    string        `json:"icon,omitempty"`
	PW      *packwiz.Meta `json:"pw,omitempty"`
	// Lost: the file isn't in the store and couldn't be kept.
	Lost bool `json:"lost,omitempty"`
}

// State is an instance's content as a snapshot keeps it.
type State struct {
	Items  []StateItem `json:"items"`
	Packs  []string    `json:"packs"`  // enabled resource packs, top first
	Shader string      `json:"shader"` // active shader pack ("" for none)
	// Shared are content folders the instance shares with others: left out.
	Shared []string `json:"shared,omitempty"`
}

// State reads an instance's content after a scan. With keep, files the
// store doesn't have yet (another launcher's folder, which is never
// adopted) are copied into it, so a rollback can put them back.
func (s *Service) State(ctx context.Context, id string, keep bool) (State, error) {
	for _, k := range Kinds {
		if err := s.Scan(ctx, id, k); err != nil {
			return State{}, err
		}
	}
	v, err := s.view(id)
	if err != nil {
		return State{}, err
	}
	st := State{Packs: append([]string{}, v.packs...), Shader: v.shader}
	shared := map[Kind]bool{}
	for _, k := range Kinds {
		if s.base(id, k) != v.game {
			shared[k] = true
			st.Shared = append(st.Shared, k.Top())
		}
	}
	for item, e := range v.idx.Entries {
		if e.Dir || e.SHA1 == "" || shared[e.Kind] {
			continue // folder packs have no blob to keep
		}
		kn := v.known[e.SHA1]
		it := StateItem{ID: item, Kind: e.Kind, Rel: e.Rel, SHA1: e.SHA1, Name: displayName(e, kn), Keys: keysOf(e, kn)}
		if ident := kn.Ident(); ident != nil && ident.Version != "" {
			it.Version = ident.Version
		} else if kn.Info != nil {
			it.Version = kn.Info.Version
		}
		if kn.Info != nil {
			it.Icon = kn.Info.Icon
		}
		if pw, ok := s.pwFor(&v, e); ok {
			it.PW = &pw
		}
		if keep {
			it.Lost = !s.stored(id, e)
		}
		st.Items = append(st.Items, it)
	}
	sort.Slice(st.Items, func(a, b int) bool { return st.Items[a].Rel < st.Items[b].Rel })
	return st, nil
}

// keysOf names an item by every project it is on and its mod id.
func keysOf(e Entry, k Known) []string {
	var out []string
	for _, src := range modplatform.Sources {
		if id := k.By(src); id != nil && id.ProjectID != "" {
			out = append(out, ScopeProject(id.Source, id.ProjectID))
		}
	}
	if e.Kind == Mods && k.Info != nil && k.Info.ID != "" {
		out = append(out, "mod:"+k.Info.ID)
	}
	return out
}

// stored makes sure the store has an item's blob, copying the file in when
// it hasn't.
func (s *Service) stored(id string, e Entry) bool {
	if _, ok := s.d.Store.Blob(e.SHA1); ok {
		return true
	}
	h, err := s.d.Store.Ingest(s.abs(id, e.Rel))
	if err != nil {
		s.d.Log.Warn("content: keeping a file for a snapshot failed", "item", e.Rel, "err", err)
		return false
	}
	return h.SHA1 == e.SHA1
}

// ValidItemRel reports whether rel can be an item of kind k: a clean path
// right inside the kind's folder (a world's datapacks for data packs).
func ValidItemRel(k Kind, rel string) bool {
	sp, ok := specs[k]
	if !ok || len(rel) > 512 || !filepath.IsLocal(rel) || path.Clean(rel) != rel || strings.ContainsAny(rel, "\\:") {
		return false
	}
	dir, name := path.Split(rel)
	if name == "" || strings.HasPrefix(name, ".") || partial(name) ||
		!strings.EqualFold(path.Ext(strings.TrimSuffix(name, Disabled)), sp.ext) {
		return false
	}
	dir = strings.TrimSuffix(dir, "/")
	if k == DataPacks {
		parts := strings.Split(dir, "/")
		return len(parts) == 3 && parts[0] == "saves" && parts[2] == "datapacks" && instance.ValidWorldFolder(parts[1])
	}
	return dir == sp.folders(nil)[0].Rel
}

// ItemOp puts one item back as a snapshot had it. Remove is the file there
// now and Put where the snapshot's blob goes (either may be ""; both name
// the file as it is, with .disabled when off).
type ItemOp struct {
	Kind   Kind
	Remove string
	Put    string
	SHA1   string
	PW     *packwiz.Meta // the item's metadata to write with Put
}

// FileOp puts one other file back: write fills a staged copy; nil deletes it.
type FileOp struct {
	Rel   string
	Write func(dst string) error
}

// Restore is everything a rollback puts back, as one change.
type Restore struct {
	Items  []ItemOp
	Files  []FileOp
	Packs  *[]string // the resource pack list (nil leaves it)
	Shader *string   // the active shader pack (nil leaves it)
	// Commit runs once every file is in place (the version, the modpack);
	// an error puts every file back.
	Commit func() error
}

// Restore puts files back all or nothing: everything is staged beside its
// target first, then swapped in, so a failure part way leaves the instance
// as it was.
func (s *Service) Restore(ctx context.Context, id string, r Restore) error {
	if s.running(id) {
		return ErrGameHasFiles
	}
	unlock := s.lock(id)
	err := s.restore(id, r)
	unlock()
	if err != nil {
		return err
	}
	for _, k := range Kinds {
		_ = s.Scan(ctx, id, k) // a failed scan runs again when a tab opens
	}
	if r.Packs != nil {
		if err := s.setPacks(ctx, id, *r.Packs); err != nil {
			s.d.Log.Warn("content: restoring the resource packs failed", "err", err)
		}
	}
	if r.Shader != nil {
		if err := s.setShader(ctx, id, *r.Shader); err != nil {
			s.d.Log.Warn("content: restoring the shader failed", "err", err)
		}
	}
	for _, k := range Kinds {
		s.emit(id, k)
	}
	return nil
}

func (s *Service) restore(id string, r Restore) error {
	v, err := s.view(id)
	if err != nil {
		return err
	}
	sw := instance.NewSwap(s.d.Instances, id)
	defer sw.Close()
	fail := func(err error) error {
		sw.Rollback()
		return err
	}
	for _, op := range r.Items {
		if op.Put == "" {
			continue
		}
		if err := sw.Stage(op.Put, func(dst string) error { return s.place(id, op.SHA1, dst) }); err != nil {
			return fail(fmt.Errorf("%s: %w", path.Base(op.Put), err))
		}
	}
	for _, f := range r.Files {
		if f.Write != nil {
			if err := sw.Stage(f.Rel, f.Write); err != nil {
				return fail(fmt.Errorf("%s: %w", f.Rel, err))
			}
		}
	}
	// Everything aside first, then everything in: two changes to one name
	// (an item taking another's file name) never collide.
	for _, op := range r.Items {
		for _, rel := range []string{op.Remove, op.Put} {
			if rel != "" {
				if err := sw.Aside(rel); err != nil {
					return fail(err)
				}
			}
		}
	}
	for _, f := range r.Files {
		if err := sw.Aside(f.Rel); err != nil {
			return fail(err)
		}
	}
	for _, op := range r.Items {
		if op.Put != "" {
			if err := sw.Put(op.Put); err != nil {
				return fail(err)
			}
		}
	}
	for _, f := range r.Files {
		if f.Write != nil {
			if err := sw.Put(f.Rel); err != nil {
				return fail(err)
			}
		}
	}
	if r.Commit != nil {
		if err := r.Commit(); err != nil {
			return fail(err)
		}
	}
	sw.Finish()
	s.restoreMeta(&v, r.Items)
	return nil
}

// restoreMeta swaps the .pw.toml files of restored items and seals their
// blobs again (deleting a hardlink on Windows clears the read-only flag
// every link shares).
func (s *Service) restoreMeta(v *view, ops []ItemOp) {
	byRel := map[string]Entry{}
	for _, e := range v.idx.Entries {
		byRel[e.Rel] = e
	}
	for _, op := range ops {
		if e, ok := byRel[op.Remove]; ok && op.Remove != "" {
			if pw, ok := s.pwFor(v, e); ok {
				_ = packwiz.Remove(s.abs(v.in.ID, path.Join(path.Dir(e.Rel), specs[e.Kind].indexIn)), pw.Slug)
			}
			_ = s.d.Store.Reseal(e.SHA1)
		}
	}
	for _, op := range ops {
		if op.Put == "" {
			continue
		}
		_ = s.d.Store.Reseal(op.SHA1)
		if op.PW != nil {
			m := *op.PW
			m.Filename = strings.TrimSuffix(path.Base(op.Put), Disabled)
			dir := s.abs(v.in.ID, path.Join(path.Dir(op.Put), specs[op.Kind].indexIn))
			if err := packwiz.Write(dir, m); err != nil {
				s.d.Log.Warn("content: restoring metadata failed", "item", op.Put, "err", err)
			}
		}
	}
}
