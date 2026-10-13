package instance

import (
	"strings"

	"quartz/internal/core/slp"
)

// Into is where a launch opens straight away: a world (its folder in saves/)
// or a server (its address). The zero value opens the title screen.
type Into struct {
	Kind   string `json:"kind"` // "" | "world" | "server"
	Target string `json:"target"`
}

// Valid reports whether i names a well-formed world folder or server address.
func (i Into) Valid() bool {
	switch i.Kind {
	case "":
		return i.Target == ""
	case "world":
		return ValidWorldFolder(i.Target)
	case "server":
		_, err := slp.Parse(i.Target)
		return err == nil
	}
	return false
}

// ValidWorldFolder reports whether w can be a folder name in saves/: one
// path element, not hidden, no separators or control characters.
func ValidWorldFolder(w string) bool {
	if w == "" || len(w) > 128 || strings.HasPrefix(w, ".") || strings.ContainsAny(w, `/\`) {
		return false
	}
	for _, r := range w {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
