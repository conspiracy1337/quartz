package snapshot

import (
	"context"
	"crypto/sha1" //nolint:gosec // content addressing, as the store does
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"quartz/internal/content"
	"quartz/internal/loader"
)

// Row is one change a rollback can undo. From is how it is now, To how the
// restore point has it.
type Row struct {
	Key string `json:"key"`
	// Kind is a content kind, or version | pack | packs | shader | files.
	Kind string `json:"kind"`
	// Change is changed | enable | disable | restore | remove for items,
	// set for the rest.
	Change      string `json:"change"`
	Name        string `json:"name"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Default     bool   `json:"default"`               // ticked to begin with
	Unavailable string `json:"unavailable,omitempty"` // why it can't be rolled back

	match     []string // scope keys naming it
	snap, cur *content.StateItem
	files     []fileChange
}

// Diff is what rolling back to a restore point changes.
type Diff struct {
	Point Summary `json:"point"`
	Rows  []Row   `json:"rows"`
	// Shared folders are left out: other instances use them too.
	Shared []string `json:"shared,omitempty"`
	// Partial folders were too big to keep whole.
	Partial []string `json:"partial,omitempty"`
	// Undoable: a rollback keeps the instance first (the history is on).
	Undoable bool `json:"undoable"`
}

// Diff compares a restore point with the instance as it is now. Every row
// starts ticked: rolling back to a point puts all of it back.
func (s *Service) Diff(ctx context.Context, inst, id string) (Diff, error) {
	unlock := s.lock(inst)
	defer unlock()
	p, newest, err := s.point(inst, id)
	if err != nil {
		return Diff{}, err
	}
	sn, err := readLayer(s.dir(inst), p.ID)
	if err != nil {
		return Diff{}, err
	}
	sn.Scope = nil
	d, err := s.diff(ctx, inst, sn)
	d.Point = summary(p, newest)
	return d, err
}

// layer reads one layer of an open session (for Undo).
func (s *Service) layer(inst, id string) (Layer, error) {
	dir := s.dir(inst)
	l, err := readLayer(dir, id)
	if err != nil {
		if _, perr := s.pointOf(inst, id); perr == nil {
			return Layer{}, ErrSquashed
		}
		return Layer{}, err
	}
	return l, nil
}

// pointOf finds the point a change (by its layer's id) belongs to.
func (s *Service) pointOf(inst, change string) (Point, error) {
	for _, p := range points(s.dir(inst)) {
		for _, c := range p.Changes {
			if c.ID == change {
				return p, nil
			}
		}
		if p.ID == change {
			return p, nil
		}
	}
	return Point{}, ErrUnknown
}

func (s *Service) diff(ctx context.Context, inst string, sn Layer) (Diff, error) {
	in, err := s.d.Instances.Get(inst)
	if err != nil {
		return Diff{}, err
	}
	cur, err := s.d.Content.State(ctx, inst, false)
	if err != nil {
		return Diff{}, err
	}
	game := s.d.Instances.GameDir(inst)
	shared := map[string]bool{}
	for _, top := range append(append([]string{}, sn.State.Shared...), cur.Shared...) {
		shared[top] = true
	}
	for _, tree := range Trees {
		if s.d.Instances.Base(inst, tree) != game {
			shared[tree] = true
		}
	}
	d := Diff{Rows: []Row{}, Partial: sn.Partial, Undoable: s.enabled()}
	for top := range shared {
		d.Shared = append(d.Shared, top)
	}
	sort.Strings(d.Shared)

	if now := (Version{Minecraft: in.Minecraft, Loader: in.Loader, LoaderVersion: in.LoaderVersion}); now != sn.Version {
		r := Row{Key: content.ScopeVersion, Kind: "version", Change: "set", Name: "Minecraft and loader",
			From: versionLabel(now), To: versionLabel(sn.Version), match: []string{content.ScopeVersion}}
		if in.ClientJar != nil && (sn.Version.Minecraft != in.Minecraft || sn.Version.Loader != "") {
			r.Unavailable = "It runs a custom client jar"
		}
		d.Rows = append(d.Rows, r)
	}
	if sn.Pack != nil && (in.Pack == nil || in.Pack.Version != sn.Pack.Version) {
		r := Row{Key: content.ScopePack, Kind: "pack", Change: "set", Name: sn.Pack.Name, To: sn.Pack.VersionName, match: []string{content.ScopePack}}
		if in.Pack != nil {
			r.From = in.Pack.VersionName
		}
		d.Rows = append(d.Rows, r)
	}
	d.Rows = append(d.Rows, s.itemRows(game, sn, cur, shared)...)
	if !shared["resourcepacks"] && !slices.Equal(sn.State.Packs, cur.Packs) {
		d.Rows = append(d.Rows, Row{Key: content.ScopePacks, Kind: "packs", Change: "set", Name: "Resource packs in use",
			From: packsLabel(cur.Packs), To: packsLabel(sn.State.Packs), Detail: strings.Join(sn.State.Packs, ", "), match: []string{content.ScopePacks}})
	}
	if !shared["shaderpacks"] && sn.State.Shader != cur.Shader {
		d.Rows = append(d.Rows, Row{Key: content.ScopeShader, Kind: "shader", Change: "set", Name: "Shader pack in use",
			From: orNone(cur.Shader), To: orNone(sn.State.Shader), match: []string{content.ScopeShader}})
	}
	if changes, err := s.fileChanges(inst, sn, shared); err == nil && len(changes) > 0 {
		d.Rows = append(d.Rows, Row{Key: content.ScopeFiles, Kind: "files", Change: "set", Name: "Config files",
			Detail: filesDetail(changes), match: []string{content.ScopeFiles}, files: changes})
	}
	scope := map[string]bool{}
	for _, k := range sn.Scope {
		scope[k] = true
	}
	for i := range d.Rows {
		r := &d.Rows[i]
		r.Default = sn.Scope == nil || slices.ContainsFunc(r.match, func(k string) bool { return scope[k] })
	}
	return d, nil
}

// itemRows pairs the layer's items with the instance's: by item (kind
// and path), then by project or mod id (an update usually renames the
// file). Ambiguous matches stay apart, as a restore and a removal.
func (s *Service) itemRows(game string, sn Layer, cur content.State, shared map[string]bool) []Row {
	keep := func(it content.StateItem) bool { return !shared[it.Kind.Top()] }
	was := slices.DeleteFunc(slices.Clone(sn.State.Items), func(it content.StateItem) bool { return !keep(it) })
	now := slices.DeleteFunc(slices.Clone(cur.Items), func(it content.StateItem) bool { return !keep(it) })
	pair := make([]int, len(was))
	used := make([]bool, len(now))
	byID := map[string]int{}
	for j, it := range now {
		byID[string(it.Kind)+"|"+it.ID] = j
	}
	for i, it := range was {
		pair[i] = -1
		if j, ok := byID[string(it.Kind)+"|"+it.ID]; ok {
			pair[i], used[j] = j, true
		}
	}
	byKey := map[string]int{}
	for j, it := range now {
		if used[j] {
			continue
		}
		for _, k := range it.Keys {
			key := string(it.Kind) + "|" + k
			if _, dup := byKey[key]; dup {
				byKey[key] = -1
			} else {
				byKey[key] = j
			}
		}
	}
	for i, it := range was {
		if pair[i] >= 0 {
			continue
		}
		for _, k := range it.Keys {
			if j, ok := byKey[string(it.Kind)+"|"+k]; ok && j >= 0 && !used[j] {
				pair[i], used[j] = j, true
				break
			}
		}
	}
	var rows []Row
	for i := range was {
		var c *content.StateItem
		if pair[i] >= 0 {
			c = &now[pair[i]]
		}
		if r, ok := s.itemRow(game, &was[i], c); ok {
			rows = append(rows, r)
		}
	}
	for j := range now {
		if !used[j] {
			r, _ := s.itemRow(game, nil, &now[j])
			rows = append(rows, r)
		}
	}
	order := map[content.Kind]int{}
	for i, k := range content.Kinds {
		order[k] = i
	}
	sort.SliceStable(rows, func(a, b int) bool {
		ka, kb := content.Kind(rows[a].Kind), content.Kind(rows[b].Kind)
		if ka != kb {
			return order[ka] < order[kb]
		}
		return strings.ToLower(rows[a].Name) < strings.ToLower(rows[b].Name)
	})
	return rows
}

// itemRow is the change that puts one item back as the layer had it
// (snap nil: added since, cur nil: removed since).
func (s *Service) itemRow(game string, snap, cur *content.StateItem) (Row, bool) {
	r := Row{snap: snap, cur: cur}
	for _, it := range []*content.StateItem{snap, cur} {
		if it == nil {
			continue
		}
		r.match = append(r.match, content.ScopeRel(it.Rel))
		r.match = append(r.match, it.Keys...)
		r.Kind, r.Name = string(it.Kind), it.Name
		if r.Icon == "" {
			r.Icon = it.Icon
		}
	}
	switch {
	case snap == nil:
		r.Key, r.Change, r.From = "item:"+cur.ID, "remove", cur.Version
	case cur == nil:
		r.Key, r.Change, r.To = "item:"+snap.ID, "restore", snap.Version
	case snap.SHA1 == cur.SHA1 && snap.Rel == cur.Rel:
		return Row{}, false
	default:
		r.Key, r.Name, r.From, r.To = "item:"+snap.ID, snap.Name, cur.Version, snap.Version
		wasOn, isOn := on(snap.Rel), on(cur.Rel)
		switch {
		case snap.SHA1 == cur.SHA1 && strings.TrimSuffix(snap.Rel, content.Disabled) == strings.TrimSuffix(cur.Rel, content.Disabled):
			r.Change = map[bool]string{true: "enable", false: "disable"}[wasOn]
		default:
			r.Change = "changed"
			if wasOn != isOn {
				r.Detail = map[bool]string{true: "and turned on", false: "and turned off"}[wasOn]
			}
		}
	}
	r.Unavailable = s.unavailable(game, snap, cur)
	return r, true
}

// unavailable says why an item can't be put back ("" when it can).
func (s *Service) unavailable(game string, snap, cur *content.StateItem) string {
	if snap != nil {
		if _, ok := s.d.Store.Blob(snap.SHA1); snap.Lost || !ok {
			return "Its old file is gone"
		}
		if snap.Kind == content.DataPacks {
			world := strings.Join(strings.SplitN(snap.Rel, "/", 3)[:2], "/")
			if info, err := os.Stat(filepath.Join(game, filepath.FromSlash(world))); err != nil || !info.IsDir() { //nolint:gosec // checked by content.ValidItemRel
				return "Its world is gone"
			}
		}
	}
	for _, it := range []*content.StateItem{snap, cur} {
		if it != nil && linked(game, path.Dir(it.Rel)) {
			return "Its folder is a link to another launcher's folder"
		}
	}
	return ""
}

// linked reports whether any folder on the way to dir is a link.
func linked(game, dir string) bool {
	p := game
	for _, part := range strings.Split(dir, "/") {
		p = filepath.Join(p, part)
		if info, err := os.Lstat(p); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func on(rel string) bool { return !strings.HasSuffix(rel, content.Disabled) }

// fileChange is one file a rollback writes back (from the layer) or
// deletes (snap nil: it wasn't there).
type fileChange struct {
	rel  string
	snap *File
}

// fileChanges compares the layer's files with the game folder: changed
// and removed ones come back, ones added since in a kept tree go. Files a
// partial tree couldn't keep are left alone.
func (s *Service) fileChanges(inst string, sn Layer, shared map[string]bool) ([]fileChange, error) {
	game, err := os.OpenRoot(s.d.Instances.GameDir(inst))
	if err != nil {
		return nil, err
	}
	defer func() { _ = game.Close() }()
	kept := map[string]*File{}
	for i := range sn.Files {
		kept[sn.Files[i].Rel] = &sn.Files[i]
	}
	var trees []string
	for _, t := range sn.Trees {
		if !shared[t] {
			trees = append(trees, t)
		}
	}
	var out []fileChange
	seen := map[string]bool{}
	for _, tree := range trees {
		partial := slices.Contains(sn.Partial, tree)
		walkFiles(game, tree, func(rel string, info fs.FileInfo) {
			seen[rel] = true
			f, ok := kept[rel]
			switch {
			case !ok && !partial:
				out = append(out, fileChange{rel: rel})
			case ok && f.Absent:
				out = append(out, fileChange{rel: rel})
			case ok && differs(game, rel, info, *f):
				out = append(out, fileChange{rel: rel, snap: f})
			}
		})
	}
	for i := range sn.Files {
		f := &sn.Files[i]
		if seen[f.Rel] {
			continue
		}
		if inTree(f.Rel, sn.Trees) && !inTree(f.Rel, trees) {
			continue // in a folder shared since
		}
		info, err := game.Lstat(f.Rel)
		switch {
		case f.Absent && err == nil:
			out = append(out, fileChange{rel: f.Rel})
		case f.Absent:
		case err != nil:
			out = append(out, fileChange{rel: f.Rel, snap: f})
		case info.Mode().IsRegular() && differs(game, f.Rel, info, *f):
			out = append(out, fileChange{rel: f.Rel, snap: f})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].rel < out[b].rel })
	return out, nil
}

// differs reports whether the file at rel isn't the kept one: an equal
// size and time means the same file, otherwise its content decides.
func differs(game *os.Root, rel string, info fs.FileInfo, f File) bool {
	if info.Size() == f.Size && info.ModTime().UnixNano() == f.ModNS {
		return false
	}
	if info.Size() != f.Size {
		return true
	}
	r, err := game.Open(rel)
	if err != nil {
		return true
	}
	defer func() { _ = r.Close() }()
	h := sha1.New() //nolint:gosec // content addressing, as the store does
	if _, err := io.Copy(h, io.LimitReader(r, maxFile+1)); err != nil {
		return true
	}
	return hex.EncodeToString(h.Sum(nil)) != f.SHA1
}

func filesDetail(changes []fileChange) string {
	back, gone := 0, 0
	for _, c := range changes {
		if c.snap == nil {
			gone++
		} else {
			back++
		}
	}
	var parts []string
	if back > 0 {
		parts = append(parts, fmt.Sprintf("%d %s back", back, plural(back, "file comes", "files come")))
	}
	if gone > 0 {
		parts = append(parts, fmt.Sprintf("%d new %s", gone, plural(gone, "file goes", "files go")))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// versionLabel is "1.21.4, Fabric 0.16.10" (the row is named Minecraft and loader).
func versionLabel(v Version) string {
	out := v.Minecraft
	if v.Loader != "" {
		out += ", " + loader.Label(loader.Kind(v.Loader), v.Minecraft, v.LoaderVersion)
	}
	return out
}

func packsLabel(packs []string) string {
	switch len(packs) {
	case 0:
		return "None"
	case 1:
		return "1 pack"
	}
	return fmt.Sprintf("%d packs", len(packs))
}

func orNone(s string) string {
	if s == "" {
		return "None"
	}
	return s
}
