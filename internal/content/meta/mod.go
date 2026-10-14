package meta

import (
	"bufio"
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"quartz/internal/core/archive"
	"quartz/internal/core/version"
)

// Nested jars (Fabric's jar-in-jar, Forge's JarJar) are read one level deep
// for the mod ids they provide, within these limits.
const (
	maxNestedJar  = 16 << 20
	maxNestedJars = 64
)

// Mod reads a mod jar's metadata. Descriptors are tried in a fixed order;
// the loaders of every descriptor present are listed, so a jar built for
// several loaders reports all of them.
func Mod(r *archive.Reader, ic Icons) (Info, error) {
	info, iconFile, err := modInfo(r)
	if err != nil {
		return Info{}, err
	}
	info.Icon = icon(r, iconFile, ic)
	info.Provides = append(info.Provides, nestedIDs(r)...)
	info.Provides = cleanIDs(info.Provides)
	return info, nil
}

type parser struct {
	name   string
	loader string
	parse  func(data []byte, r Source) (Info, string, error)
}

var parsers = []parser{
	{"fabric.mod.json", FormatFabric, parseFabric},
	{"quilt.mod.json", FormatQuilt, parseQuilt},
	{"META-INF/neoforge.mods.toml", FormatNeoForge, parseForgeTOML},
	{"META-INF/mods.toml", FormatForge, parseForgeTOML},
	{"mcmod.info", "", parseModInfo},
}

func modInfo(r Source) (Info, string, error) {
	var info Info
	var iconFile string
	var loaders []string
	found := false
	for _, p := range parsers {
		if !r.Has(p.name) {
			continue
		}
		if p.loader != "" {
			loaders = append(loaders, p.loader)
		}
		if found {
			continue
		}
		data, err := r.Read(p.name, maxText)
		if err != nil {
			continue
		}
		i, ic, err := p.parse(data, r)
		if err != nil {
			continue
		}
		if i.Format == "" {
			i.Format = p.loader
		}
		info, iconFile, found = i, ic, true
	}
	if !found {
		return Info{}, "", ErrNotContent
	}
	if len(loaders) == 0 && info.Format == FormatModInfo {
		loaders = []string{FormatForge}
	}
	info.Loaders = loaders
	info.ID = cleanID(info.ID)
	info.Name = clean(info.Name, maxName)
	if info.Name == "" {
		info.Name = info.ID
	}
	info.Version = clean(info.Version, maxName)
	info.Description = clean(info.Description, maxDesc)
	info.Authors = cleanList(info.Authors, maxAuthors)
	info.Deps = cleanDeps(info.Deps)
	info.Game = cleanRange(info.Game)
	return info, iconFile, nil
}

// ---- Fabric ----

type fabricJSON struct {
	ID          string                     `json:"id"`
	Version     string                     `json:"version"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Authors     []json.RawMessage          `json:"authors"`
	Icon        json.RawMessage            `json:"icon"`
	Depends     map[string]json.RawMessage `json:"depends"`
	Breaks      map[string]json.RawMessage `json:"breaks"`
	Provides    []string                   `json:"provides"`
	Environment string                     `json:"environment"`
	Custom      struct {
		ModMenu struct {
			Badges []string `json:"badges"`
		} `json:"modmenu"`
	} `json:"custom"`
}

func parseFabric(data []byte, _ Source) (Info, string, error) {
	var f fabricJSON
	if err := decodeJSON(data, &f); err != nil {
		return Info{}, "", err
	}
	if f.ID == "" {
		return Info{}, "", ErrNotContent
	}
	info := Info{ID: f.ID, Name: f.Name, Version: f.Version, Description: f.Description, Provides: f.Provides,
		Library: slices.Contains(f.Custom.ModMenu.Badges, "library"), Side: side(f.Environment)}
	for _, a := range f.Authors {
		info.Authors = append(info.Authors, person(a))
	}
	for id, raw := range f.Depends {
		info.addDep(id, Required, fabricRange(raw))
	}
	for id, raw := range f.Breaks {
		info.addDep(id, Breaks, fabricRange(raw))
	}
	return info, iconPath(f.Icon), nil
}

// addDep records a relation; a requirement on Minecraft itself is the range
// of game versions the mod runs on.
func (i *Info) addDep(id string, k DepKind, r string) {
	if k == Required && strings.EqualFold(id, "minecraft") {
		i.Game = r
		return
	}
	i.Deps = append(i.Deps, Dep{ID: id, Kind: k, Range: r})
}

// fabricRange reads a version predicate: a string, or a list of
// alternatives. Anything else can't be judged.
func fabricRange(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var list []string
	if json.Unmarshal(raw, &list) != nil {
		return version.Unknown
	}
	for i, v := range list {
		if list[i] = strings.TrimSpace(v); list[i] == version.Any {
			return version.Any
		}
	}
	return strings.Join(list, "||")
}

// person reads an author: a name, or an object with a name.
func person(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &o)
	return o.Name
}

// iconPath picks an icon: a path, or the largest of a {"64x64": path} map.
func iconPath(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var sizes map[string]string
	if json.Unmarshal(raw, &sizes) != nil {
		return ""
	}
	best, bestSize := "", -1
	for k, v := range sizes {
		n, _ := strconv.Atoi(strings.SplitN(k, "x", 2)[0])
		if n > bestSize || (n == bestSize && v < best) {
			best, bestSize = v, n
		}
	}
	return best
}

// ---- Quilt ----

type quiltJSON struct {
	Loader struct {
		ID       string            `json:"id"`
		Version  string            `json:"version"`
		Provides []json.RawMessage `json:"provides"`
		Depends  []json.RawMessage `json:"depends"`
		Breaks   []json.RawMessage `json:"breaks"`
		Metadata struct {
			Name         string            `json:"name"`
			Description  string            `json:"description"`
			Contributors map[string]string `json:"contributors"`
			Icon         json.RawMessage   `json:"icon"`
		} `json:"metadata"`
	} `json:"quilt_loader"`
	Minecraft struct {
		Environment string `json:"environment"`
	} `json:"minecraft"`
}

func parseQuilt(data []byte, _ Source) (Info, string, error) {
	var q quiltJSON
	if err := decodeJSON(data, &q); err != nil {
		return Info{}, "", err
	}
	l := q.Loader
	if l.ID == "" {
		return Info{}, "", ErrNotContent
	}
	info := Info{ID: l.ID, Name: l.Metadata.Name, Version: l.Version, Description: l.Metadata.Description, Side: side(q.Minecraft.Environment)}
	names := make([]string, 0, len(l.Metadata.Contributors))
	for name := range l.Metadata.Contributors {
		names = append(names, name)
	}
	sort.Strings(names)
	info.Authors = names
	for _, p := range l.Provides {
		if ref := quiltRef(p); ref.ID != "" {
			info.Provides = append(info.Provides, ref.ID)
		}
	}
	for _, d := range l.Depends {
		if ref := quiltRef(d); ref.ID != "" && !ref.Optional {
			info.addDep(ref.ID, Required, ref.versions())
		}
	}
	for _, d := range l.Breaks {
		if ref := quiltRef(d); ref.ID != "" {
			info.addDep(ref.ID, Breaks, ref.versions())
		}
	}
	return info, iconPath(l.Metadata.Icon), nil
}

// side maps a Fabric or Quilt environment ("*", "client", "server",
// "dedicated_server") to a Side.
func side(env string) string {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "client":
		return SideClient
	case "server", "dedicated_server":
		return SideServer
	}
	return ""
}

// quiltDep is a dependency reference: "id", or an object with versions.
type quiltDep struct {
	ID       string          `json:"id"`
	Optional bool            `json:"optional"`
	Versions json.RawMessage `json:"versions"`
}

// quiltRef reads "id" or {"id": …, "optional": …, "versions": …}.
func quiltRef(raw json.RawMessage) quiltDep {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return quiltDep{ID: s}
	}
	var o quiltDep
	_ = json.Unmarshal(raw, &o)
	return o
}

// versions is the reference's range; Quilt's {"any": …}/{"all": …} objects
// can't be judged.
func (d quiltDep) versions() string {
	if len(d.Versions) == 0 {
		return ""
	}
	return fabricRange(d.Versions)
}

// ---- Forge and NeoForge (mods.toml) ----

func parseForgeTOML(data []byte, r Source) (Info, string, error) {
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		info, iconFile := salvageTOML(data)
		if info.ID == "" {
			return Info{}, "", err
		}
		info.Version = jarVersion(info.Version, r)
		return info, iconFile, nil
	}
	mods, _ := doc["mods"].([]any)
	if len(mods) == 0 {
		return Info{}, "", ErrNotContent
	}
	mod, _ := mods[0].(map[string]any)
	pick := func(key string) any {
		if v, ok := mod[key]; ok {
			return v
		}
		return doc[key]
	}
	info := Info{ID: str(mod["modId"]), Name: str(mod["displayName"]), Description: str(mod["description"])}
	info.Version = jarVersion(str(mod["version"]), r)
	info.Authors = splitAuthors(pick("authors"))
	for _, m := range mods[1:] {
		if mm, ok := m.(map[string]any); ok {
			info.Provides = append(info.Provides, str(mm["modId"]))
		}
	}
	for _, d := range forgeDeps(doc["dependencies"], info.ID) {
		info.addDep(d.ID, d.Kind, d.Range)
	}
	if only, _ := pick("clientSideOnly").(bool); only {
		info.Side = SideClient
	}
	return info, str(pick("logoFile")), nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func splitAuthors(v any) []string {
	switch a := v.(type) {
	case string:
		return strings.Split(a, ",")
	case []any:
		var out []string
		for _, e := range a {
			out = append(out, str(e))
		}
		return out
	}
	return nil
}

// forgeDeps reads [[dependencies.<modId>]]: Forge marks requirements with
// mandatory=true, NeoForge with type="required" (and "incompatible").
func forgeDeps(v any, self string) []Dep {
	tables, _ := v.(map[string]any)
	list, ok := tables[self].([]any)
	if !ok {
		// Some mods key the table by another id; take the first one.
		keys := make([]string, 0, len(tables))
		for k := range tables {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			list, _ = tables[keys[0]].([]any)
		}
	}
	var deps []Dep
	for _, e := range list {
		d, _ := e.(map[string]any)
		id, r := str(d["modId"]), str(d["versionRange"])
		mandatory, _ := d["mandatory"].(bool)
		switch typ := strings.ToLower(str(d["type"])); {
		case typ == "required" || (typ == "" && mandatory):
			deps = append(deps, Dep{ID: id, Kind: Required, Range: r})
		case typ == "incompatible":
			deps = append(deps, Dep{ID: id, Kind: Breaks, Range: r})
		}
	}
	return deps
}

var tomlField = regexp.MustCompile(`(?m)^\s*(modId|version|displayName|logoFile|authors)\s*=\s*["']([^"'\r\n]*)["']`)

// salvageTOML pulls the basics out of a mods.toml TOML parsers reject (Forge's
// NightConfig is more forgiving than the spec).
func salvageTOML(data []byte) (Info, string) {
	found := map[string]string{}
	for _, m := range tomlField.FindAllSubmatch(data, -1) {
		if _, dup := found[string(m[1])]; !dup {
			found[string(m[1])] = string(m[2])
		}
	}
	info := Info{ID: found["modId"], Name: found["displayName"], Version: found["version"], Partial: true,
		Authors: splitAuthors(found["authors"])}
	return info, found["logoFile"]
}

// jarVersion resolves Forge's ${file.jarVersion} from the jar's manifest.
func jarVersion(v string, r Source) string {
	if v != "${file.jarVersion}" {
		return v
	}
	mf, err := r.Read("META-INF/MANIFEST.MF", maxText)
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(bytes.NewReader(mf))
	for sc.Scan() {
		if k, val, ok := strings.Cut(sc.Text(), ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Implementation-Version") {
			if val = strings.TrimSpace(val); !strings.Contains(val, "archiveVersion") {
				return val
			}
		}
	}
	return ""
}

// ---- mcmod.info (Forge 1.12 and older) ----

type modInfoEntry struct {
	ModID        string   `json:"modid"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	AuthorList   []string `json:"authorList"`
	Authors      []string `json:"authors"`
	LogoFile     string   `json:"logoFile"`
	RequiredMods []string `json:"requiredMods"`
	Dependencies []string `json:"dependencies"`
}

func parseModInfo(data []byte, _ Source) (Info, string, error) {
	var list []modInfoEntry
	if err := decodeJSON(data, &list); err != nil || len(list) == 0 {
		var wrapped struct {
			ModList  []modInfoEntry `json:"modList"`
			Modlist2 []modInfoEntry `json:"modlist"`
		}
		if err := decodeJSON(data, &wrapped); err != nil {
			return Info{}, "", err
		}
		list = append(wrapped.ModList, wrapped.Modlist2...)
	}
	if len(list) == 0 {
		return Info{}, "", ErrNotContent
	}
	e := list[0]
	info := Info{Format: FormatModInfo, ID: strings.TrimPrefix(e.ModID, "mod_"), Name: e.Name, Version: e.Version, Description: e.Description,
		Authors: append(e.AuthorList, e.Authors...)}
	if info.Name == "Example Mod" {
		info.Name = ""
	}
	// requiredMods are hard requirements; "dependencies" only order loading,
	// unless written FML's way ("required-after:Baubles@[1.5,)").
	reqs := slices.Clone(e.RequiredMods)
	for _, d := range e.Dependencies {
		if strings.HasPrefix(d, "required-") {
			reqs = append(reqs, d)
		}
	}
	for _, d := range reqs {
		if _, after, ok := strings.Cut(d, ":"); ok {
			d = after
		}
		d, r, _ := strings.Cut(d, "@")
		info.addDep(strings.TrimPrefix(d, "mod_"), Required, r)
	}
	for _, m := range list[1:] {
		info.Provides = append(info.Provides, m.ModID)
	}
	return info, e.LogoFile, nil
}

// ---- nested jars ----

func nestedIDs(r *archive.Reader) []string {
	var ids []string
	jars := append(r.Glob("META-INF/jars/", ".jar"), r.Glob("META-INF/jarjar/", ".jar")...)
	for i, name := range jars {
		if i == maxNestedJars {
			break
		}
		data, err := r.Read(name, maxNestedJar)
		if err != nil {
			continue
		}
		inner, err := archive.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			continue
		}
		info, _, err := modInfo(inner)
		if err != nil {
			continue
		}
		ids = append(ids, info.ID)
		ids = append(ids, info.Provides...)
	}
	return ids
}

// ---- cleaning ----

var idRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,64}$`)

func cleanID(id string) string {
	id = strings.TrimSpace(id)
	if !idRe.MatchString(id) {
		return ""
	}
	return strings.ToLower(id)
}

func cleanIDs(ids []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id = cleanID(id); id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// cleanRange keeps a version range short and printable; a stranger one is
// kept as unreadable rather than dropped (dropping would mean "any").
func cleanRange(r string) string {
	r = strings.TrimSpace(r)
	if len(r) > 128 || strings.ContainsFunc(r, func(c rune) bool { return c < ' ' || c > '~' }) {
		return version.Unknown
	}
	return r
}

// Dependencies every instance satisfies by itself.
var builtin = map[string]bool{"minecraft": true, "java": true, "fabricloader": true, "quilt_loader": true,
	"forge": true, "neoforge": true, "fml": true, "mcp": true, "mod_minecraftforge": true}

func cleanDeps(in []Dep) []Dep {
	var out []Dep
	seen := map[Dep]bool{}
	for _, d := range in {
		d.ID = cleanID(d.ID)
		d.Range = cleanRange(d.Range)
		if d.ID == "" || builtin[d.ID] || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind+DepKind(out[i].ID) < out[j].Kind+DepKind(out[j].ID) })
	if len(out) > maxDeps {
		out = out[:maxDeps]
	}
	return out
}
