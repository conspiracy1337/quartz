package content

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"quartz/internal/core/fsx"
	"quartz/internal/modplatform"
)

// Counts is how many items of each kind an instance has, from its index.
func (s *Service) Counts(id string) map[Kind]int {
	out := map[Kind]int{Mods: 0, ResourcePacks: 0, Shaders: 0, DataPacks: 0}
	ix, err := s.idx.get(id)
	if err != nil {
		return out
	}
	for _, e := range ix.Get().Entries {
		out[e.Kind]++
	}
	return out
}

// PendingUpdate is one update the last check found.
type PendingUpdate struct {
	Item string `json:"item"`
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Pending lists the updates the last check found that still apply.
func (s *Service) Pending(id string) []PendingUpdate {
	v, err := s.view(id)
	if err != nil {
		return nil
	}
	var out []PendingUpdate
	for item, u := range v.idx.Updates {
		e, ok := v.idx.Entries[item]
		if !ok {
			continue
		}
		if pw, ok := s.pwFor(&v, e); ok && pw.Pin {
			continue
		}
		k := v.known[e.SHA1]
		icon := ""
		if k.Info != nil {
			icon = k.Info.Icon
		}
		out = append(out, PendingUpdate{Item: item, Kind: e.Kind, Name: displayName(e, k), Icon: icon, From: u.From, To: u.Version})
	}
	sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name) })
	return out
}

// InstanceMissing is Home's missing-dependency alert for one instance.
type InstanceMissing struct {
	InstanceID string `json:"instanceId"`
	Missing
}

// Missing lists instances whose mods lack a dependency, from their indexes.
func (s *Service) Missing() []InstanceMissing {
	var out []InstanceMissing
	for _, in := range s.d.Instances.List() {
		ix, err := s.idx.get(in.ID)
		if err != nil {
			continue
		}
		if m := ix.Get().Missing; m != nil {
			out = append(out, InstanceMissing{InstanceID: in.ID, Missing: *m})
		}
	}
	return out
}

// TabInfo is what a content tab says besides its rows.
type TabInfo struct {
	// ShaderMod is "Iris 1.8.1" or "Oculus 1.7.0", or "" without either.
	ShaderMod string `json:"shaderMod"`
	// Worlds lists the worlds data packs can go into.
	Worlds []string `json:"worlds"`
}

// Info describes an instance's content tabs.
func (s *Service) Info(id string) TabInfo {
	v, err := s.view(id)
	if err != nil {
		return TabInfo{}
	}
	var t TabInfo
	if m := s.shaderMod(v); m.id != "" {
		t.ShaderMod = strings.TrimSpace(m.name + " " + m.version)
	}
	if root, err := os.OpenRoot(s.base(id, DataPacks)); err == nil {
		for _, f := range worlds(root) {
			t.Worlds = append(t.Worlds, f.Group)
		}
		_ = root.Close()
	}
	sort.Strings(t.Worlds)
	return t
}

// Quick is the launch check: a scan of the mods (stat-only when nothing is
// new) and a one-line summary with any missing dependency.
func (s *Service) Quick(ctx context.Context, id string) (count int, detail string, warn bool, err error) {
	if err := s.Scan(ctx, id, Mods); err != nil {
		return 0, "", false, err
	}
	ix, err := s.idx.get(id)
	if err != nil {
		return 0, "", false, err
	}
	cur := ix.Get()
	for _, e := range entriesOf(cur, Mods) {
		if !strings.HasSuffix(e.Rel, Disabled) {
			count++
		}
	}
	if m := cur.Missing; m != nil {
		return count, fmt.Sprintf("%s needs %s", m.Mod, m.Dep), true, nil
	}
	return count, fmt.Sprintf("%d %s", count, noun(Mods, count)), false, nil
}

// Refs yields every blob an instance (or a trashed one) still uses, for the
// store's garbage collection.
func (s *Service) Refs(yield func(sha1 string)) error {
	s.idx.flush()
	var dirs []string
	for _, in := range s.d.Instances.List() {
		dirs = append(dirs, s.d.Instances.Dir(in.ID))
	}
	dirs = append(dirs, s.d.Instances.TrashDirs()...)
	for _, dir := range dirs {
		data, err := fsx.ReadCapped(filepath.Join(dir, indexFile), 32<<20)
		if err != nil {
			continue
		}
		var i Index
		if err := json.Unmarshal(data, &i); err != nil {
			return fmt.Errorf("content: %s is unreadable, so nothing is collected: %w", filepath.Join(dir, indexFile), err)
		}
		for _, e := range i.Entries {
			if e.SHA1 != "" {
				yield(e.SHA1)
			}
		}
		for _, r := range i.Removed {
			for _, it := range r.Items {
				if it.Entry.SHA1 != "" {
					yield(it.Entry.SHA1)
				}
			}
		}
	}
	return nil
}

// Forget flushes and drops an instance's cached index before its folder
// moves (to the trash, or elsewhere).
func (s *Service) Forget(id string) { s.idx.forget(id) }

// Path is where an item is on disk.
func (s *Service) Path(id, item string) (string, error) {
	e, err := s.entry(id, item)
	if err != nil {
		return "", err
	}
	return s.abs(id, e.Rel), nil
}

// Installed is an identified item an instance has.
type Installed struct {
	Item string `json:"item"`
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
	// Modrinth and CurseForge are the item's version on each platform that
	// knows it.
	Modrinth   *Ident `json:"modrinth,omitempty"`
	CurseForge *Ident `json:"curseforge,omitempty"`
}

// Ident is the item's identification on src.
func (i Installed) Ident(src modplatform.Source) *Ident {
	if src == modplatform.CurseForge {
		return i.CurseForge
	}
	return i.Modrinth
}

// Installed lists an instance's identified items, from its index (no scan).
func (s *Service) Installed(id string) []Installed {
	v, err := s.view(id)
	if err != nil {
		return nil
	}
	var out []Installed
	for item, e := range v.idx.Entries {
		k := v.known[e.SHA1]
		if k.Modrinth == nil && k.CurseForge == nil {
			continue
		}
		out = append(out, Installed{Item: item, Kind: e.Kind, Name: displayName(e, k), Modrinth: k.Modrinth, CurseForge: k.CurseForge})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Item < out[b].Item })
	return out
}
