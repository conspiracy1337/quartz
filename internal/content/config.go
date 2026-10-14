package content

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"quartz/internal/core/optfile"
	"quartz/internal/core/version"
)

// Two small game files hold content state the game owns: options.txt
// (resource pack order) and Iris/Oculus' properties (the active shader).
// Both are edited key by key; every other line stays byte for byte.

// ---- options.txt ----

// optionsPath is the game's options file.
func optionsPath(game string) string { return filepath.Join(game, "options.txt") }

// Mods that copy a modpack's default options.txt into place on the first
// start, but only while there is none: until then their copy is the file to
// edit, or creating options.txt would throw the pack's defaults away.
var defaultOptions = []string{"config/yosbr/options.txt", "config/defaultoptions/options.txt"}

// legacyPacks reports whether Minecraft names packs the old way: before 1.13
// the list holds bare file names and nothing built in. Versions that can't be
// read (snapshots) count as new.
func legacyPacks(mc string) bool {
	in, ok := version.Satisfies(version.Fabric, "<1.13", mc)
	return ok && in
}

// packID is how options.txt names a pack in resourcepacks/.
func packID(name string, legacy bool) string {
	if legacy {
		return name
	}
	return "file/" + name
}

// ErrOptionsDamaged refuses to rewrite a pack list that can't be read, which
// would lose the built-in packs it holds.
var ErrOptionsDamaged = errors.New("content: the resource pack list in options.txt is damaged. Fix or delete options.txt first")

var quoted = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

// readPackList reads a JSON list of pack ids. A cut-off list (a crash while
// the game wrote it) still yields its complete entries; ok is false only
// when there is a value and nothing at all can be read from it.
func readPackList(l optfile.File, key string) (list []string, ok bool) {
	v, found := l.Get(key)
	if !found || strings.TrimSpace(v) == "" {
		return nil, true
	}
	if json.Unmarshal([]byte(v), &list) == nil {
		return list, true
	}
	for _, m := range quoted.FindAllStringSubmatch(v, -1) {
		var id string
		if json.Unmarshal([]byte(`"`+m[1]+`"`), &id) == nil {
			list = append(list, id)
		}
	}
	return list, len(list) > 0
}

func encodePackList(ids []string) string {
	if ids == nil {
		ids = []string{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

// orderPacks rewrites the resource pack list with the wanted packs, given
// top (highest priority) first, as the game stores them: lowest first.
// Built-in packs (vanilla, fabric, programmer_art, …) keep their place: each
// stays just above the file pack it sat on, so a new file list leaves them
// where the player put them. Packs the game would call incompatible are also
// listed in incompatibleResourcePacks, or it would drop them on start.
func orderPacks(l *optfile.File, wanted []string, incompatible map[string]bool, worldVersion int, legacy bool) error {
	old, ok := readPackList(*l, "resourcePacks")
	if !ok {
		return ErrOptionsDamaged
	}
	keep := map[string]bool{}
	for _, w := range wanted {
		keep[packID(w, legacy)] = true
	}
	above := map[string][]string{} // file pack id ("" = the bottom) → built-ins right above it
	if !legacy {
		anchor := ""
		for _, id := range old {
			switch {
			case strings.HasPrefix(id, "file/"):
				if keep[id] {
					anchor = id
				}
			default:
				above[anchor] = append(above[anchor], id)
			}
		}
		if !l.Exist && len(old) == 0 {
			above[""] = []string{"vanilla"}
		}
	}
	list := append([]string{}, above[""]...)
	var incompat []string
	for i := len(wanted) - 1; i >= 0; i-- {
		id := packID(wanted[i], legacy)
		list = append(list, id)
		list = append(list, above[id]...)
		if incompatible[wanted[i]] {
			incompat = append(incompat, id)
		}
	}
	if !l.Exist && worldVersion > 0 {
		l.Set("version", strconv.Itoa(worldVersion))
	}
	l.Set("resourcePacks", encodePackList(list))
	l.Set("incompatibleResourcePacks", encodePackList(incompat))
	return nil
}

// enabledPacks lists the enabled packs in resourcepacks/, top first.
func enabledPacks(l optfile.File, legacy bool) []string {
	list, _ := readPackList(l, "resourcePacks")
	var out []string
	for i := len(list) - 1; i >= 0; i-- {
		if name, ok := strings.CutPrefix(list[i], "file/"); ok {
			out = append(out, name)
		} else if legacy {
			out = append(out, list[i])
		}
	}
	return out
}

// ---- Iris / Oculus properties ----

// shaderConfig is the properties file of the shader mod an instance has,
// relative to the instance (config/ may be shared).
func shaderConfig(oculus bool) string {
	if oculus {
		return "config/oculus.properties"
	}
	return "config/iris.properties"
}

// activeShader reads the selected shader pack ("" when shaders are off).
func activeShader(l optfile.File) string {
	if v, _ := l.Get("enableShaders"); strings.EqualFold(strings.TrimSpace(v), "false") {
		return ""
	}
	v, _ := l.Get("shaderPack")
	return v
}

func setShader(l *optfile.File, name string) {
	if name == "" {
		l.Set("enableShaders", "false")
		return
	}
	l.Set("shaderPack", name)
	l.Set("enableShaders", "true")
}
