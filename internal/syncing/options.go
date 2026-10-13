package syncing

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"quartz/internal/core/keycodes"
	"quartz/internal/core/optfile"
)

// Game options sync key by key, as Pandora does, with what Pandora misses:
//   - each instance keeps its own version line (the game upgrades options
//     from it) and its resource pack list (the Resource packs tab owns it);
//   - keybinds change format at 1.13 (LWJGL2 codes before, GLFW names
//     after) and lang at 1.11 (en_US before, en_us after).
// The shared copy holds every value in the 1.13+ form. A launch writes the
// shared values for the keys the instance's file has; after the run, the
// keys the run changed go back to the shared copy, so the newest change of
// each key wins.

// ownKeys never sync.
var ownKeys = map[string]bool{"version": true, "resourcePacks": true, "incompatibleResourcePacks": true}

// format is how one instance's options.txt writes values.
type format struct {
	legacyKeys bool // LWJGL2 key codes (before 1.13)
	upperLang  bool // en_US (before 1.11)
}

// formatOf reads the format from the file when it has keybinds, else from
// the Minecraft version ("1.12.2" is legacy; snapshots and "26.1" aren't).
func formatOf(mc string, f optfile.File) format {
	minor := -1
	if rest, ok := strings.CutPrefix(mc, "1."); ok {
		end := strings.IndexFunc(rest, func(r rune) bool { return !unicode.IsDigit(r) })
		if end < 0 {
			end = len(rest)
		}
		minor, _ = strconv.Atoi(rest[:end])
	}
	fm := format{legacyKeys: minor >= 0 && minor < 13, upperLang: minor >= 0 && minor < 11}
	for _, k := range f.Keys() {
		if strings.HasPrefix(k, "key_") {
			v, _ := f.Get(k)
			_, err := strconv.Atoi(v)
			fm.legacyKeys = err == nil
			break
		}
	}
	return fm
}

// canonical turns a value into the shared (1.13+) form.
func canonical(key, value string) string {
	switch {
	case strings.HasPrefix(key, "key_"):
		if k, ok := keycodes.Parse(value); ok {
			return k.Name
		}
	case key == "lang":
		return strings.ToLower(value)
	}
	return value
}

// localize turns a shared value into fm's form; false skips the key.
func (fm format) localize(key, value string) (string, bool) {
	switch {
	case strings.HasPrefix(key, "key_"):
		k, ok := keycodes.ByName(value)
		if !ok {
			return value, !fm.legacyKeys // a name the table doesn't know is only safe after 1.13
		}
		return keycodes.Format(k, fm.legacyKeys)
	case key == "lang" && fm.upperLang:
		if lang, region, ok := strings.Cut(value, "_"); ok {
			return lang + "_" + strings.ToUpper(region), true
		}
	}
	return value, true
}

// canonicalValues are a file's syncable values in the shared form.
func canonicalValues(f optfile.File) map[string]string {
	out := map[string]string{}
	for k, v := range f.Values() {
		if !ownKeys[k] && validValue(k, v) {
			out[k] = canonical(k, v)
		}
	}
	return out
}

func validValue(k, v string) bool {
	return len(k) <= 128 && !strings.ContainsAny(k+v, "\r\n\x00")
}

// mergeIn writes the shared values into an instance's file: for the keys it
// has, or all of them into a file that doesn't exist yet.
func mergeIn(dst *optfile.File, shared map[string]string, fm format) bool {
	changed := false
	fresh := !dst.Exist
	for _, k := range sortedKeys(shared) {
		if ownKeys[k] || (!fresh && !dst.Has(k)) {
			continue
		}
		v, ok := fm.localize(k, shared[k])
		if !ok {
			continue
		}
		if old, had := dst.Get(k); !had || old != v {
			dst.Set(k, v)
			changed = true
		}
	}
	return changed
}

// changedSince lists the values a run changed: keys whose canonical value
// differs from the snapshot taken at launch.
func changedSince(before, after map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range after {
		if old, ok := before[k]; !ok || old != v {
			out[k] = v
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string { return slices.Sorted(maps.Keys(m)) }
