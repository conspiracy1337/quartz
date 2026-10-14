package content

import (
	"context"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"quartz/internal/content/meta"
	"quartz/internal/content/packwiz"
	"quartz/internal/core/optfile"
	"quartz/internal/instance"
)

// Row is one item as a content tab shows it.
type Row struct {
	ID      string  `json:"id"`
	Kind    Kind    `json:"kind"`
	Name    string  `json:"name"`
	Sub     string  `json:"sub"`
	Version string  `json:"version"`
	Icon    string  `json:"icon,omitempty"` // sha256 of a stored icon
	Source  string  `json:"source"`         // modrinth | curseforge | file
	Enabled bool    `json:"enabled"`
	Locked  bool    `json:"locked"`
	Group   string  `json:"group,omitempty"` // the world, for data packs
	Status  *Status `json:"status,omitempty"`
	// Project is the item's page on its platform, when identified.
	Project string `json:"project,omitempty"`
	// Ref names the project for the launcher's own project page.
	Ref string `json:"ref,omitempty"`
	// ClientOnly marks a mod a local server leaves out.
	ClientOnly bool `json:"clientOnly,omitempty"`
}

// Status is the one badge a row shows, most important first.
type Status struct {
	Kind    string `json:"kind"` // problem | update | locked | outdated | world | unknown
	Version string `json:"version,omitempty"`
	Label   string `json:"label,omitempty"`
	Tone    string `json:"tone,omitempty"`
	World   string `json:"world,omitempty"`
}

// view gathers everything rows of one instance are built from.
type view struct {
	in      instance.Instance
	game    string
	idx     Index
	known   map[string]Known
	pw      map[string]map[string]packwiz.Meta // index folder → lower filename → meta
	options string                             // the options.txt to edit (see optionsFile)
	legacy  bool                               // packs named the pre-1.13 way
	packs   []string                           // enabled resource packs, top first
	shader  string                             // active shader pack
	formats meta.ClientFormats
	fmtOK   bool
}

// List scans one kind and returns its rows.
func (s *Service) List(ctx context.Context, id string, k Kind) ([]Row, error) {
	if err := s.Scan(ctx, id, k); err != nil {
		return nil, err
	}
	v, err := s.view(id)
	if err != nil {
		return nil, err
	}
	return s.rows(v, k), nil
}

func (s *Service) view(id string) (view, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return view{}, err
	}
	if err := s.d.Instances.CheckGameDir(id); err != nil {
		return view{}, err
	}
	ix, err := s.idx.get(id)
	if err != nil {
		return view{}, err
	}
	v := view{in: in, game: s.d.Instances.GameDir(id), idx: ix.Get(), pw: map[string]map[string]packwiz.Meta{}}
	sha := make([]string, 0, len(v.idx.Entries))
	for _, e := range v.idx.Entries {
		if e.SHA1 != "" {
			sha = append(sha, e.SHA1)
		}
	}
	v.known = s.cat.many(sha)
	v.options, v.legacy = s.optionsFile(id, v.game), legacyPacks(in.Minecraft)
	opts, _ := optfile.Read(v.options, ":")
	v.packs = enabledPacks(opts, v.legacy)
	if v.idx.Pending.Packs != nil {
		v.packs = v.idx.Pending.Packs
	}
	mod := s.shaderMod(v).id
	props, _ := optfile.Read(s.abs(v.in.ID, shaderConfig(mod)), "=")
	v.shader = activeShader(props, mod)
	if v.idx.Pending.Shader != nil {
		v.shader = *v.idx.Pending.Shader
	}
	v.formats, v.fmtOK = s.clientFormats(in.Minecraft)
	return v, nil
}

// optionsFile is the options.txt to edit: the game's own, or before the
// first start a defaults mod's copy (see defaultOptions).
func (s *Service) optionsFile(id, game string) string {
	own := optionsPath(game)
	if _, err := os.Stat(own); err == nil {
		return own
	}
	for _, rel := range defaultOptions {
		p := s.abs(id, rel)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	return own
}

// pwFor returns the packwiz metadata describing an entry, if any.
func (s *Service) pwFor(v *view, e Entry) (packwiz.Meta, bool) {
	dir := path.Join(path.Dir(e.Rel), specs[e.Kind].indexIn)
	m, ok := v.pw[dir]
	if !ok {
		m, _ = packwiz.ReadDir(s.abs(v.in.ID, dir))
		v.pw[dir] = m
	}
	pw, ok := m[strings.ToLower(strings.TrimSuffix(path.Base(e.Rel), Disabled))]
	return pw, ok
}

func (s *Service) rows(v view, k Kind) []Row {
	entries := entriesOf(v.idx, k)
	var problems map[string]Problem
	if k == Mods {
		_, problems = s.problems(v.in.ID, v.idx)
	}
	rank := map[string]int{}
	for i, name := range v.packs {
		rank[name] = i
	}
	out := make([]Row, 0, len(entries))
	for id, e := range entries {
		out = append(out, s.row(&v, id, e, problems))
	}
	sort.SliceStable(out, func(a, b int) bool {
		ra, rb := out[a], out[b]
		if k == ResourcePacks && ra.Enabled != rb.Enabled {
			return ra.Enabled
		}
		if k == ResourcePacks && ra.Enabled {
			return rank[base(entries[ra.ID])] < rank[base(entries[rb.ID])]
		}
		if ra.Group != rb.Group {
			return ra.Group < rb.Group
		}
		return strings.ToLower(ra.Name) < strings.ToLower(rb.Name)
	})
	return out
}

// base is an entry's file name without .disabled.
func base(e Entry) string { return strings.TrimSuffix(path.Base(e.Rel), Disabled) }

func (s *Service) row(v *view, id string, e Entry, problems map[string]Problem) Row {
	k := v.known[e.SHA1]
	info := k.Info
	if e.Dir {
		info = e.Info
	}
	if info == nil {
		info = &meta.Info{}
	}
	pw, hasPW := s.pwFor(v, e)
	ident := k.Ident()
	r := Row{ID: id, Kind: e.Kind, Name: displayName(e, k), Version: info.Version, Icon: info.Icon, Source: "file",
		Group: e.Group, Locked: hasPW && pw.Pin}
	switch e.Kind {
	case ResourcePacks:
		r.Enabled = slices.Contains(v.packs, base(e))
	case Shaders:
		r.Enabled = v.shader != "" && v.shader == base(e)
	default:
		r.Enabled = !strings.HasSuffix(e.Rel, Disabled)
	}
	switch {
	case ident != nil:
		r.Source, r.Version, r.Project = ident.Source, orElse(ident.Version, r.Version), projectURL(e.Kind, ident)
		r.Ref = k.Ref().String()
	case hasPW && pw.Modrinth != nil:
		r.Source, r.Version = "modrinth", orElse(pw.VersionNumber, r.Version)
	case hasPW && pw.CurseForge != nil:
		r.Source, r.Version = "curseforge", orElse(pw.VersionNumber, r.Version)
	}
	r.Sub = sub(e.Kind, info, ident)
	unknown := r.Source == "file" && !e.Dir && k.IdentAt > 0
	if unknown && r.Sub == "" {
		r.Sub = "Not identified"
	}
	r.Status = s.status(v, id, e, info, problems, r, unknown)
	r.ClientOnly = e.Kind == Mods && !e.Dir && s.clientOnly(v, e)
	return r
}

func (s *Service) status(v *view, id string, e Entry, info *meta.Info, problems map[string]Problem, r Row, unknown bool) *Status {
	if p, ok := problems[id]; ok {
		return &Status{Kind: "problem", Label: p.Label, Tone: p.Tone}
	}
	if !e.Dir && v.known[e.SHA1].Bad {
		// A broken or truncated file: say so instead of listing it as unknown.
		return &Status{Kind: "problem", Label: "Can't read this file", Tone: "danger"}
	}
	if u, ok := v.idx.Updates[id]; ok && !r.Locked {
		return &Status{Kind: "update", Version: u.Version}
	}
	if r.Locked {
		return &Status{Kind: "locked"}
	}
	if label := s.formatLabel(v, e.Kind, info); label != "" {
		return &Status{Kind: "outdated", Label: label}
	}
	if e.Kind == DataPacks && e.Group != "" {
		return &Status{Kind: "world", World: e.Group}
	}
	if unknown {
		return &Status{Kind: "unknown"}
	}
	return nil
}

func (s *Service) formatLabel(v *view, k Kind, info *meta.Info) string {
	if !v.fmtOK || info.Pack == nil {
		return ""
	}
	switch k {
	case ResourcePacks:
		return meta.Label("resource", info.Pack, v.formats.Resource)
	case DataPacks:
		if v.formats.HasData {
			return meta.Label("data", info.Pack, v.formats.Data)
		}
	}
	return ""
}

// displayName prefers the platform's title, then the file's own name, then
// the file name.
func displayName(e Entry, k Known) string {
	if id := k.Ident(); id != nil && id.Title != "" {
		return id.Title
	}
	if k.Info != nil && k.Info.Name != "" {
		return k.Info.Name
	}
	if e.Info != nil && e.Info.Name != "" {
		return e.Info.Name
	}
	name := base(e)
	if !e.Dir {
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	return name
}

// sub is the second line: authors for mods, shaders and data packs, the
// description for resource packs (as the design shows them).
func sub(k Kind, info *meta.Info, ident *Ident) string {
	authors := strings.Join(info.Authors, ", ")
	if ident != nil && ident.Author != "" {
		authors = ident.Author
	}
	if k == ResourcePacks && info.Description != "" {
		return info.Description
	}
	if authors != "" {
		return authors
	}
	return info.Description
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

var curseClass = map[Kind]string{Mods: "mc-mods", ResourcePacks: "texture-packs", Shaders: "shaders", DataPacks: "data-packs"}

func projectURL(k Kind, id *Ident) string {
	switch {
	case id.Source == "modrinth" && id.ProjectID != "":
		return "https://modrinth.com/project/" + orElse(id.Slug, id.ProjectID)
	case id.Source == "curseforge" && id.Slug != "":
		return "https://www.curseforge.com/minecraft/" + curseClass[k] + "/" + id.Slug
	}
	return ""
}

type shaderMod struct{ id, name, version string }

// shaderMod finds the enabled shader mod: Iris or Oculus, else OptiFine (its
// jar, or OptiFabric that loads it), as XMCL also supports. "" when none.
func (s *Service) shaderMod(v view) shaderMod {
	var optifine shaderMod
	for _, e := range entriesOf(v.idx, Mods) {
		if strings.HasSuffix(e.Rel, Disabled) {
			continue
		}
		info := v.known[e.SHA1].Info
		switch {
		case info != nil && (info.ID == "iris" || info.ID == "oculus"):
			return shaderMod{id: info.ID, name: map[string]string{"iris": "Iris", "oculus": "Oculus"}[info.ID], version: info.Version}
		case info != nil && (info.ID == "optifabric" || info.Name == "OptiFine"),
			strings.HasPrefix(strings.ToLower(path.Base(e.Rel)), "optifine"):
			optifine = shaderMod{id: "optifine", name: "OptiFine"}
		}
	}
	return optifine
}

var clientFormatCache sync.Map // minecraft version → meta.ClientFormats (only successes)

func (s *Service) clientFormats(mc string) (meta.ClientFormats, bool) {
	if v, ok := clientFormatCache.Load(mc); ok {
		return v.(meta.ClientFormats), true
	}
	if s.d.ClientJar == nil {
		return meta.ClientFormats{}, false
	}
	f, ok := meta.ClientPack(s.d.ClientJar(mc))
	if ok {
		clientFormatCache.Store(mc, f)
	}
	return f, ok
}
