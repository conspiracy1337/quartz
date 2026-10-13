package content

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"quartz/internal/core/optfile"
)

// Two small game files hold content state the game owns: options.txt
// (resource pack order) and Iris/Oculus' properties (the active shader).
// Both are edited key by key; every other line stays byte for byte.

// ---- options.txt ----

// optionsPath is the game's options file.
func optionsPath(game string) string { return filepath.Join(game, "options.txt") }

// packID is how options.txt names a pack in resourcepacks/.
func packID(name string) string { return "file/" + name }

func readPackList(l optfile.File, key string) []string {
	v, ok := l.Get(key)
	if !ok {
		return nil
	}
	var out []string
	if json.Unmarshal([]byte(v), &out) != nil {
		return nil
	}
	return out
}

func encodePackList(ids []string) string {
	if ids == nil {
		ids = []string{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

// orderPacks rewrites the resource pack list: built-in packs (vanilla,
// fabric, …) keep their place at the bottom, then the wanted packs, given
// top (highest priority) first, as the game stores them: lowest first.
// Packs the game would call incompatible are also listed in
// incompatibleResourcePacks, or it would drop them on start.
func orderPacks(l *optfile.File, wanted []string, incompatible map[string]bool, worldVersion int) {
	var list []string
	for _, id := range readPackList(*l, "resourcePacks") {
		if !strings.HasPrefix(id, "file/") {
			list = append(list, id)
		}
	}
	if !l.Exist && len(list) == 0 {
		list = []string{"vanilla"}
	}
	var incompat []string
	for i := len(wanted) - 1; i >= 0; i-- {
		list = append(list, packID(wanted[i]))
		if incompatible[wanted[i]] {
			incompat = append(incompat, packID(wanted[i]))
		}
	}
	if !l.Exist && worldVersion > 0 {
		l.Set("version", strconv.Itoa(worldVersion))
	}
	l.Set("resourcePacks", encodePackList(list))
	l.Set("incompatibleResourcePacks", encodePackList(incompat))
}

// enabledPacks lists the enabled packs in resourcepacks/, top first.
func enabledPacks(l optfile.File) []string {
	list := readPackList(l, "resourcePacks")
	var out []string
	for i := len(list) - 1; i >= 0; i-- {
		if name, ok := strings.CutPrefix(list[i], "file/"); ok {
			out = append(out, name)
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
