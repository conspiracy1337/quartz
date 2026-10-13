package instance

import (
	"strings"
	"unicode"
	"unicode/utf16"
)

// tints are the instance tile colours (look.ts TINTS in the UI).
var tints = [...]string{"#3ecf8e", "#e2a03f", "#6b8afd", "#b48cf2", "#c76bd8", "#9aa3ad", "#5bc0be", "#e86f6f", "#f2d45c", "#7cc7e8"}

// ArtFor is the tile the UI gives a new instance called name (look.ts
// artOf): a tint picked from the name and its first letter or digit, so
// instances made in Go look the same as ones made on screen.
func ArtFor(name string) Art {
	var h uint32
	for _, r := range name {
		c := r
		if r > 0xffff { // JavaScript's charCodeAt(0): the high surrogate
			c, _ = utf16.EncodeRune(r)
		}
		h = h*31 + uint32(c) //nolint:gosec // a code unit
	}
	initial := "?"
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			initial = strings.ToUpper(string(r))
			break
		}
	}
	return Art{Background: tints[h%uint32(len(tints))], Initial: initial}
}
