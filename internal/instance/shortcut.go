package instance

import (
	"path/filepath"
	"regexp"
)

// Shortcut is a desktop or app-menu shortcut made for the instance (see
// package shortcuts), recorded so it can be removed with the instance,
// brought back when it is restored, and rewritten when the launcher moves.
type Shortcut struct {
	Where   string `json:"where"`             // "desktop" | "menu"
	Path    string `json:"path"`              // the file; "" while the instance is trashed
	Account string `json:"account,omitempty"` // a fixed account id; "" is the default account
	Into    Into   `json:"into"`
}

// MaxShortcuts caps the shortcuts one instance keeps.
const MaxShortcuts = 16

var shortcutAccount = regexp.MustCompile(`^[0-9a-f]{32}$`)

// cleanShortcuts drops malformed records (the file may be hand-edited).
func cleanShortcuts(list []Shortcut) []Shortcut {
	var out []Shortcut
	for _, s := range list {
		if (s.Where != "desktop" && s.Where != "menu") || (s.Path != "" && !filepath.IsAbs(s.Path)) ||
			(s.Account != "" && !shortcutAccount.MatchString(s.Account)) || !s.Into.Valid() {
			continue
		}
		if out = append(out, s); len(out) == MaxShortcuts {
			break
		}
	}
	return out
}
