package instance

import "strings"

// idSlug is the readable part of an instance's id and folder name: lowercase
// ASCII letters and digits joined by single dashes, at most about 40 long.
// It stays ASCII because Java tools still trip over other characters in a
// game folder's path; accented Latin letters are written as their base
// letter instead of being dropped, and a name with none left is "instance".
func idSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case latinFold[r] != "":
			b.WriteString(latinFold[r])
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	if slug := strings.Trim(b.String(), "-"); slug != "" {
		return slug
	}
	return "instance"
}

// latinFold spells the lowercase letters of Latin-1 and Latin Extended-A
// (the European languages) with ASCII letters.
var latinFold = func() map[rune]string {
	m := map[rune]string{}
	for letters, ascii := range map[string]string{
		"àáâãäåāăą": "a", "æ": "ae", "çćĉċč": "c", "ďđð": "d", "èéêëēĕėęě": "e", "ĝğġģ": "g", "ĥħ": "h",
		"ìíîïĩīĭįı": "i", "ĳ": "ij", "ĵ": "j", "ķĸ": "k", "ĺļľŀł": "l", "ñńņňŉŋ": "n", "òóôõöøōŏő": "o",
		"œ": "oe", "ŕŗř": "r", "śŝşšſ": "s", "ß": "ss", "ţťŧ": "t", "þ": "th", "ùúûüũūŭůűų": "u",
		"ŵ": "w", "ýÿŷ": "y", "źżž": "z",
	} {
		for _, r := range letters {
			m[r] = ascii
		}
	}
	return m
}()
