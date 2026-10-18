package markdown

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
)

// isPunct reports ASCII punctuation, the characters a backslash escapes.
func isPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// isSpace is goldmark's util.IsSpace: the bytes that end a link destination
// or separate words.
func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func isSpaceOrTab(c byte) bool { return c == ' ' || c == '\t' }

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

func isPunctRune(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) }

// isSpaceRune is goldmark's util.IsSpaceRune (its ASCII spaces are all
// unicode.IsSpace).
func isSpaceRune(r rune) bool { return unicode.IsSpace(r) }

func isBlank(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isSpace(s[i]) {
			return false
		}
	}
	return true
}

func trimSpace(s string) string { return strings.Trim(s, " \t\n\r") }

// escapeHTML writes s with the characters HTML gives meaning escaped.
func escapeHTML(b *strings.Builder, s string) {
	last := 0
	for i := 0; i < len(s); i++ {
		var rep string
		switch s[i] {
		case '&':
			rep = "&amp;"
		case '<':
			rep = "&lt;"
		case '>':
			rep = "&gt;"
		case '"':
			rep = "&quot;"
		case 0:
			rep = "\uFFFD"
		default:
			continue
		}
		b.WriteString(s[last:i])
		b.WriteString(rep)
		last = i + 1
	}
	b.WriteString(s[last:])
}

// validRune is goldmark's util.ToValidRune.
func validRune(v uint64) rune {
	if v == 0 || v > utf8.MaxRune || !utf8.ValidRune(rune(v)) {
		return utf8.RuneError
	}
	return rune(v)
}

// entityAt decodes the character reference at s[i] ('&'): a known HTML5
// name, &#digits; (1–7) or &#xhex; (1–6). It returns the text and the
// length read, or ok false when s[i:] is no reference.
func entityAt(s string, i int) (string, int, bool) {
	j := i + 1
	if j < len(s) && s[j] == '#' {
		j++
		if j < len(s) && (s[j] == 'x' || s[j] == 'X') {
			k := j + 1
			for k < len(s) && isHex(s[k]) {
				k++
			}
			if k > j+1 && k < len(s) && s[k] == ';' && k-j-1 < 7 {
				v, _ := strconv.ParseUint(s[j+1:k], 16, 32)
				return string(validRune(v)), k + 1 - i, true
			}
			return "", 0, false
		}
		k := j
		for k < len(s) && s[k] >= '0' && s[k] <= '9' {
			k++
		}
		if k > j && k < len(s) && s[k] == ';' && k-j < 8 {
			v, _ := strconv.ParseUint(s[j:k], 10, 32)
			return string(validRune(v)), k + 1 - i, true
		}
		return "", 0, false
	}
	k := j
	for k < len(s) && isAlnum(s[k]) {
		k++
	}
	if k == j || k >= len(s) || s[k] != ';' {
		return "", 0, false
	}
	if v, ok := namedEntity(s[i : k+1]); ok {
		return v, k + 1 - i, true
	}
	return "", 0, false
}

// namedEntity decodes "&name;" with the entity table x/net/html already
// carries. It decodes names without the semicolon too (HTML's legacy
// rule), so a result that kept the semicolon means the full name is unknown.
func namedEntity(ref string) (string, bool) {
	v := xhtml.UnescapeString(ref)
	if v == ref || v != ";" && strings.HasSuffix(v, ";") {
		return "", false
	}
	return v, true
}

// unescape resolves backslash escapes and character references in text, as
// goldmark's HTML writer does when it renders text.
func unescape(s string) string {
	if strings.IndexByte(s, '\\') < 0 && strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) && isPunct(s[i+1]) {
				b.WriteString(s[last:i])
				last = i + 1
				i++
			}
		case '&':
			if v, n, ok := entityAt(s, i); ok {
				b.WriteString(s[last:i])
				b.WriteString(v)
				i += n - 1
				last = i + 1
			}
		}
	}
	b.WriteString(s[last:])
	return b.String()
}

// unescapePunct drops the backslash of each escaped punctuation character
// (goldmark's util.UnescapePunctuations).
func unescapePunct(s string) string {
	if strings.IndexByte(s, '\\') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && isPunct(s[i+1]) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// resolveNumeric and resolveNames are goldmark's two passes over a link
// destination, run one after the other (so "&#38;amp;" ends as "&").
func resolveNumeric(s string) string {
	if strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '&' || i+2 >= len(s) || s[i+1] != '#' {
			continue
		}
		j := i + 2
		hex := s[j] == 'x' || s[j] == 'X'
		if hex {
			j++
		}
		k := j
		for k < len(s) && (hex && isHex(s[k]) || !hex && s[k] >= '0' && s[k] <= '9') {
			k++
		}
		if k == j || k >= len(s) || s[k] != ';' || !hex && k-j >= 8 {
			continue
		}
		base := 0 // goldmark parses decimal references with base 0
		if hex {
			base = 16
		}
		v, _ := strconv.ParseUint(s[j:k], base, 32)
		b.WriteString(s[last:i])
		b.WriteRune(validRune(v))
		i = k
		last = k + 1
	}
	b.WriteString(s[last:])
	return b.String()
}

func resolveNames(s string) string {
	if strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '&' {
			continue
		}
		k := i + 1
		for k < len(s) && isAlnum(s[k]) {
			k++
		}
		if k == i+1 || k >= len(s) || s[k] != ';' {
			continue
		}
		if v, ok := namedEntity(s[i : k+1]); ok {
			b.WriteString(s[last:i])
			b.WriteString(v)
			i = k
			last = k + 1
		}
	}
	b.WriteString(s[last:])
	return b.String()
}

// urlSafe are the bytes goldmark's util.URLEscape keeps as they are.
var urlSafe = func() (t [256]bool) {
	for _, c := range []byte("!#$&'()*+,-./0123456789:;=?@ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz~") {
		t[c] = true
	}
	return t
}()

// urlEscape percent-encodes what may not stand in a URL, keeping existing
// %XX escapes (goldmark's util.URLEscape). resolve first applies backslash
// escapes and character references, as goldmark does for link destinations.
func urlEscape(s string, resolve bool) string {
	if resolve {
		s = resolveNames(resolveNumeric(unescapePunct(s)))
	}
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case urlSafe[c]:
			b.WriteByte(c)
			i++
		case c == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]):
			b.WriteString(s[i : i+3])
			i += 3
		case c == ' ':
			b.WriteString("%20")
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				b.WriteByte(c) // invalid UTF-8 stays as it is
				i++
				continue
			}
			for k := i; k < i+size; k++ {
				b.WriteByte('%')
				b.WriteByte(hexDigits[s[k]>>4])
				b.WriteByte(hexDigits[s[k]&15])
			}
			i += size
		}
	}
	return b.String()
}

// normalizeLabel matches link labels case-insensitively with collapsed
// whitespace. Unicode case folding is approximated by lower(upper(r)), with
// the one common full folding (ß, ẞ → ss) added.
func normalizeLabel(s string) string {
	s = strings.Trim(s, " \t\n\v\f\r")
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }), " ")
	s = strings.ToLower(strings.ToUpper(s))
	return strings.NewReplacer("ß", "ss", "ẞ", "ss").Replace(s)
}
