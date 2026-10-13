package modplatform

import (
	"errors"
	"strings"
)

// Ref names a project on one or both platforms, as the UI passes it around:
// "mr:AANobbMI", "cf:394468" or "mr:AANobbMI+cf:394468".
type Ref struct {
	Modrinth   string
	CurseForge string
}

// ErrRef is returned for a malformed reference.
var ErrRef = errors.New("modplatform: invalid project reference")

// RefOf names one project on one platform.
func RefOf(src Source, id string) Ref {
	if src == CurseForge {
		return Ref{CurseForge: id}
	}
	return Ref{Modrinth: id}
}

// ParseRef reads a reference; every id must be valid.
func ParseRef(s string) (Ref, error) {
	var r Ref
	if s == "" || len(s) > 80 {
		return r, ErrRef
	}
	for _, part := range strings.Split(s, "+") {
		src, id, ok := strings.Cut(part, ":")
		if !ok || !ValidID(id) {
			return Ref{}, ErrRef
		}
		switch {
		case src == "mr" && r.Modrinth == "":
			r.Modrinth = id
		case src == "cf" && r.CurseForge == "" && isDigits(id):
			r.CurseForge = id
		default:
			return Ref{}, ErrRef
		}
	}
	return r, nil
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// String formats the reference (Modrinth first).
func (r Ref) String() string {
	var parts []string
	if r.Modrinth != "" {
		parts = append(parts, "mr:"+r.Modrinth)
	}
	if r.CurseForge != "" {
		parts = append(parts, "cf:"+r.CurseForge)
	}
	return strings.Join(parts, "+")
}

// ID is the project's id on src, or "".
func (r Ref) ID(src Source) string {
	if src == CurseForge {
		return r.CurseForge
	}
	return r.Modrinth
}

// Sources lists the platforms the reference names, Modrinth first.
func (r Ref) Sources() []Source {
	var out []Source
	if r.Modrinth != "" {
		out = append(out, Modrinth)
	}
	if r.CurseForge != "" {
		out = append(out, CurseForge)
	}
	return out
}

// Key is the project's identity on one platform, e.g. "modrinth:AANobbMI".
func Key(src Source, id string) string { return string(src) + ":" + id }
