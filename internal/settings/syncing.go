package settings

import (
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"quartz/internal/core/keycodes"
)

// Syncing is what can be shared between instances (the Syncing page). The
// instances that join each target are on the instances themselves.
type Syncing struct {
	// Enabled lists the target ids turned on: built-in ones such as
	// "options" or "saves", and custom ones ("c-…").
	Enabled []string     `json:"enabled"`
	Custom  []SyncCustom `json:"custom"`
	// Keys are the shared keybinds, written into joined instances on launch.
	Keys []SharedKey `json:"keys"`
}

// SyncCustom is a path inside the instance folder shared like a built-in target.
type SyncCustom struct {
	ID   string `json:"id"`
	Path string `json:"path"` // slash-separated, relative, no trailing slash
	Dir  bool   `json:"dir"`
}

// SharedKey is one shared keybind: the game's id (options.txt key_<ID>) and
// the 1.13+ name of the key it is bound to.
type SharedKey struct {
	ID   string `json:"id"`   // key.attack, key.zoomify.zoom …
	Key  string `json:"key"`  // key.keyboard.c, key.mouse.4 …
	Name string `json:"name"` // "Zoom (Zoomify)", as named when it was added
}

var (
	syncIDRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	// KeybindIDRe is what an options.txt keybind id may look like.
	KeybindIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:\-]{1,128}$`)
)

const (
	maxSyncCustom  = 64
	maxSharedKeys  = 128
	maxSyncPathLen = 200
)

// ErrBadSyncPath is returned for a custom path the launcher won't share.
var ErrBadSyncPath = errors.New("settings: use a relative path inside the instance folder, outside mods, logs and the launcher's own folders")

// syncReserved are folders whose content the launcher or the game manages
// per instance; sharing them would break both.
var syncReserved = []string{"mods", ".quartz", "logs", "crash-reports", "versions", "natives", "libraries", "assets"}

// CleanSyncPath validates a custom sync path typed by the user and returns
// it in canonical form: slash-separated, relative, without a trailing slash.
func CleanSyncPath(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	p = strings.TrimSuffix(p, "/")
	if p == "" || len(p) > maxSyncPathLen || strings.ContainsFunc(p, unicode.IsControl) ||
		strings.HasPrefix(p, "/") || strings.Contains(p, ":") || strings.ContainsAny(p, `*?"<>|`) {
		return "", ErrBadSyncPath
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return "", ErrBadSyncPath // also Windows' trailing dots and spaces
		}
	}
	if path.Clean(p) != p {
		return "", ErrBadSyncPath
	}
	top := strings.ToLower(strings.SplitN(p, "/", 2)[0])
	if slices.Contains(syncReserved, top) {
		return "", ErrBadSyncPath
	}
	return p, nil
}

func (y Syncing) normalize() Syncing {
	var out Syncing
	for _, id := range y.Enabled {
		if syncIDRe.MatchString(id) && !slices.Contains(out.Enabled, id) {
			out.Enabled = append(out.Enabled, id)
		}
	}
	for _, c := range y.Custom {
		p, err := CleanSyncPath(c.Path)
		if err != nil || !strings.HasPrefix(c.ID, "c-") || !syncIDRe.MatchString(c.ID) || len(out.Custom) == maxSyncCustom ||
			slices.ContainsFunc(out.Custom, func(o SyncCustom) bool { return o.ID == c.ID || o.Path == p }) {
			continue
		}
		out.Custom = append(out.Custom, SyncCustom{ID: c.ID, Path: p, Dir: c.Dir})
	}
	for _, k := range y.Keys {
		if _, ok := keycodes.ByName(k.Key); !ok || !KeybindIDRe.MatchString(k.ID) || len(out.Keys) == maxSharedKeys ||
			slices.ContainsFunc(out.Keys, func(o SharedKey) bool { return o.ID == k.ID }) {
			continue
		}
		if len([]rune(k.Name)) > 128 || strings.ContainsFunc(k.Name, unicode.IsControl) {
			k.Name = ""
		}
		out.Keys = append(out.Keys, k)
	}
	return out
}
