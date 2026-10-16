package syncing

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"quartz/internal/core/archive"
	"quartz/internal/core/keycodes"
	"quartz/internal/core/optfile"
	"quartz/internal/instance"
	"quartz/internal/settings"
)

// Shared keybinds are picked from the keys the instances' options.txt files
// have (vanilla and mod keys alike), named from the game's and the mods'
// English lang files, the way the game's Controls screen names them.

// ErrNoKey is returned for a keybind or key the launcher doesn't know.
var ErrNoKey = errors.New("syncing: unknown key")

// maxLang caps one lang file read from a jar.
const maxLang = 1 << 20

// Keybind is one shared keybind on the Syncing page.
type Keybind struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"` // the key's label: "Left Ctrl"
}

// KeyChoice is a keybind that can be shared ("Add key").
type KeyChoice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"` // "Minecraft" or the mod's name
	Key      string `json:"key"`      // its current key's label
	Shared   bool   `json:"shared"`
	key      string // its current key's 1.13+ name
}

// Keybinds lists the shared keybinds.
func (s *Service) Keybinds() []Keybind {
	keys := s.settings().Keys
	out := make([]Keybind, 0, len(keys))
	for _, k := range keys {
		name := k.Name
		if name == "" {
			name = k.ID
		}
		out = append(out, Keybind{ID: k.ID, Name: name, Key: label(k.Key)})
	}
	return out
}

func label(name string) string {
	if k, ok := keycodes.ByName(name); ok {
		return k.Label
	}
	return name
}

// KeyCatalog lists every keybind the instances have, most recently played
// instance first for the current key.
func (s *Service) KeyCatalog() []KeyChoice {
	current := map[string]string{} // id → key name
	var order []string
	insts := s.c.Instances.List()
	for _, in := range insts {
		f, err := optfile.Read(filepath.Join(s.c.Instances.GameDir(in.ID), optionsFile), ":")
		if err != nil || !f.Exist {
			continue
		}
		values := f.Values()
		for _, line := range f.Keys() {
			id, ok := strings.CutPrefix(line, "key_")
			if _, seen := current[id]; !ok || seen || !settings.KeybindIDRe.MatchString(id) {
				continue
			}
			k, ok := keycodes.Parse(values[line])
			if !ok {
				k, _ = keycodes.ByName(keycodes.Unknown)
			}
			current[id] = k.Name
			order = append(order, id)
		}
	}
	names := s.keyNames(insts, order)
	shared := map[string]bool{}
	for _, k := range s.settings().Keys {
		shared[k.ID] = true
	}
	out := make([]KeyChoice, 0, len(order))
	for _, id := range order {
		n := names[id]
		if n.name == "" {
			n.name, n.mod = id, "Other"
		}
		out = append(out, KeyChoice{ID: id, Name: n.name, Category: n.mod, Key: label(current[id]), Shared: shared[id], key: current[id]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := rank(a.Category), rank(b.Category); ra != rb {
			return ra < rb
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out
}

// rank puts Minecraft's own keys first and unnamed ones last.
func rank(category string) int {
	switch category {
	case "Minecraft":
		return 0
	case "Other":
		return 2
	}
	return 1
}

type keyName struct{ name, mod string }

// keyNames names keybinds: Minecraft's from the newest client jar that has
// a lang file, a mod's as "Name (Mod)". Jars are only opened while some id
// is still unnamed.
func (s *Service) keyNames(insts []instance.Instance, ids []string) map[string]keyName {
	out := map[string]keyName{}
	missing := func() bool { return len(out) < len(ids) }
	for _, in := range insts {
		if !missing() {
			break
		}
		if s.c.ClientJar == nil {
			break
		}
		lang := s.lang.of(s.c.ClientJar(in.Minecraft))
		if len(lang) == 0 {
			continue
		}
		for _, id := range ids {
			if n, ok := lang[id]; ok {
				out[id] = keyName{n, "Minecraft"}
			}
		}
		break
	}
	for _, in := range insts {
		if !missing() || s.c.ModJars == nil {
			break
		}
		for _, jar := range s.c.ModJars(in.ID) {
			lang := s.lang.of(jar.Path)
			for _, id := range ids {
				if _, named := out[id]; !named {
					if n, ok := lang[id]; ok {
						out[id] = keyName{n + " (" + jar.Name + ")", jar.Name}
					}
				}
			}
			if !missing() {
				break
			}
		}
	}
	return out
}

// AddKey shares a keybind, starting from its current key.
func (s *Service) AddKey(id string) (Keybind, error) {
	var pick *KeyChoice
	for _, c := range s.KeyCatalog() {
		if c.ID == id {
			pick = &c
			break
		}
	}
	if pick == nil {
		return Keybind{}, ErrNoKey
	}
	if pick.Shared {
		return Keybind{ID: id, Name: pick.Name, Key: pick.Key}, nil
	}
	s.editSettings(func(y *settings.Syncing) {
		y.Keys = append(y.Keys, settings.SharedKey{ID: id, Key: pick.key, Name: pick.Name})
	})
	s.changed()
	return Keybind{ID: id, Name: pick.Name, Key: pick.Key}, nil
}

// SetKey binds a shared keybind to the key the browser reported
// (KeyboardEvent.code, or "Mouse<button>").
func (s *Service) SetKey(id, code string) (Keybind, error) {
	k, ok := keycodes.ByBrowser(code)
	if !ok {
		return Keybind{}, ErrNoKey
	}
	var out Keybind
	found := false
	s.editSettings(func(y *settings.Syncing) {
		for i := range y.Keys {
			if y.Keys[i].ID == id {
				y.Keys[i].Key = k.Name
				out, found = Keybind{ID: id, Name: y.Keys[i].Name, Key: k.Label}, true
			}
		}
	})
	if !found {
		return Keybind{}, ErrNoKey
	}
	s.changed()
	return out, nil
}

// RemoveKey stops sharing a keybind; instances keep the key they have.
func (s *Service) RemoveKey(id string) error {
	s.editSettings(func(y *settings.Syncing) {
		y.Keys = slices.DeleteFunc(y.Keys, func(k settings.SharedKey) bool { return k.ID == id })
	})
	s.changed()
	return nil
}

// ---- lang files ----

// langCache keeps the key names of each jar, by path, size and time.
type langCache struct {
	mu sync.Mutex
	m  map[string]langEntry
}

type langEntry struct {
	size, mod int64
	names     map[string]string
}

// of returns the "key.*" names in a jar's English lang files.
func (c *langCache) of(jar string) map[string]string {
	info, err := os.Stat(jar)
	if err != nil {
		return nil
	}
	c.mu.Lock()
	if e, ok := c.m[jar]; ok && e.size == info.Size() && e.mod == info.ModTime().UnixNano() {
		c.mu.Unlock()
		return e.names
	}
	c.mu.Unlock()
	names := readLang(jar)
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]langEntry{}
	}
	c.m[jar] = langEntry{info.Size(), info.ModTime().UnixNano(), names}
	c.mu.Unlock()
	return names
}

// readLang reads assets/*/lang/en_us.json (1.13+) or en_us.lang / en_US.lang
// (older), keeping only keybind names.
func readLang(jar string) map[string]string {
	r, err := archive.Open(jar)
	if err != nil {
		return nil
	}
	defer func() { _ = r.Close() }()
	out := map[string]string{}
	for _, name := range append(r.Glob("assets/", "/lang/en_us.json"), append(r.Glob("assets/", "/lang/en_us.lang"), r.Glob("assets/", "/lang/en_US.lang")...)...) {
		data, err := r.Read(name, maxLang)
		if err != nil {
			continue
		}
		var m map[string]string
		if strings.HasSuffix(name, ".json") {
			if json.Unmarshal(data, &m) != nil {
				continue
			}
		} else {
			m = parseLangLines(data)
		}
		for k, v := range m {
			if strings.HasPrefix(k, "key.") && !strings.HasPrefix(k, "key.categories.") && len(v) <= 128 {
				out[k] = v
			}
		}
	}
	return out
}

// parseLangLines reads the old key=value lang format.
func parseLangLines(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok && !strings.HasPrefix(k, "#") {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}
