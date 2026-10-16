package modplatform

import (
	"errors"
	"strings"
	"unicode"
)

// Ref names a project on one or more platforms, as the UI passes it around:
// "mr:AANobbMI", "cf:394468", "nr:nrcmini" or "mr:AANobbMI+cf:394468".
type Ref struct {
	Modrinth   string
	CurseForge string
	NoRisk     string
}

// ErrRef is returned for a malformed reference.
var ErrRef = errors.New("modplatform: invalid project reference")

// prefixes are each platform's tag in a reference.
var prefixes = map[Source]string{Modrinth: "mr", CurseForge: "cf", NoRisk: "nr"}

// RefOf names one project on one platform.
func RefOf(src Source, id string) Ref {
	var r Ref
	if p := r.slot(src); p != nil {
		*p = id
	}
	return r
}

func (r *Ref) slot(src Source) *string {
	switch src {
	case Modrinth:
		return &r.Modrinth
	case CurseForge:
		return &r.CurseForge
	case NoRisk:
		return &r.NoRisk
	}
	return nil
}

// ParseRef reads a reference; every id must be valid.
func ParseRef(s string) (Ref, error) {
	var r Ref
	if s == "" || len(s) > 80 {
		return r, ErrRef
	}
	for _, part := range strings.Split(s, "+") {
		tag, id, ok := strings.Cut(part, ":")
		if !ok || !ValidID(id) {
			return Ref{}, ErrRef
		}
		var slot *string
		for src, p := range prefixes {
			if p == tag {
				slot = r.slot(src)
				if src == CurseForge && !isDigits(id) {
					return Ref{}, ErrRef
				}
			}
		}
		if slot == nil || *slot != "" {
			return Ref{}, ErrRef
		}
		*slot = id
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

// String formats the reference in Sources order.
func (r Ref) String() string {
	var parts []string
	for _, src := range r.Sources() {
		parts = append(parts, prefixes[src]+":"+r.ID(src))
	}
	return strings.Join(parts, "+")
}

// ID is the project's id on src, or "".
func (r Ref) ID(src Source) string {
	if p := r.slot(src); p != nil {
		return *p
	}
	return ""
}

// With is the reference with the project's id on src added.
func (r Ref) With(src Source, id string) Ref {
	if p := r.slot(src); p != nil {
		*p = id
	}
	return r
}

// Sources lists the platforms the reference names, in Sources order.
func (r Ref) Sources() []Source {
	var out []Source
	for _, src := range Sources {
		if r.ID(src) != "" {
			out = append(out, src)
		}
	}
	return out
}

// Key is the project's identity on one platform, e.g. "modrinth:AANobbMI".
func Key(src Source, id string) string { return string(src) + ":" + id }

// Normalize is how a title is compared across platforms: lower case,
// letters and digits only ("Xaero's Minimap" → "xaerosminimap").
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
