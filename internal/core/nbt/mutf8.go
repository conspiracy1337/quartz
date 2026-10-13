package nbt

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// NBT strings are Java's modified UTF-8: NUL is two bytes (C0 80) and characters
// outside the Basic Multilingual Plane are two three-byte surrogates. Plain
// ASCII without NUL is the same in both, which is nearly every string.

// EncodeString converts a Go string to modified UTF-8.
func EncodeString(s string) string {
	if isPlain(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch {
		case r == 0:
			b.WriteString("\xc0\x80")
		case r < 0x80:
			b.WriteByte(byte(r))
		case r < 0x800:
			b.WriteByte(byte(0xc0 | r>>6))
			b.WriteByte(byte(0x80 | r&0x3f))
		case r < 0x10000:
			put3(&b, r)
		default:
			hi, lo := utf16.EncodeRune(r)
			put3(&b, hi)
			put3(&b, lo)
		}
	}
	return b.String()
}

func put3(b *strings.Builder, r rune) {
	b.WriteByte(byte(0xe0 | r>>12))
	b.WriteByte(byte(0x80 | (r>>6)&0x3f))
	b.WriteByte(byte(0x80 | r&0x3f))
}

// DecodeString converts modified UTF-8 to a Go string. Malformed bytes become U+FFFD.
func DecodeString(s string) string {
	if isPlain(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	var pending rune // a high surrogate waiting for its low half
	flush := func() {
		if pending != 0 {
			b.WriteRune(utf8.RuneError)
			pending = 0
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		var r rune
		switch {
		case c < 0x80:
			r, i = rune(c), i+1
		case c&0xe0 == 0xc0 && i+1 < len(s) && s[i+1]&0xc0 == 0x80:
			r, i = rune(c&0x1f)<<6|rune(s[i+1]&0x3f), i+2
		case c&0xf0 == 0xe0 && i+2 < len(s) && s[i+1]&0xc0 == 0x80 && s[i+2]&0xc0 == 0x80:
			r, i = rune(c&0x0f)<<12|rune(s[i+1]&0x3f)<<6|rune(s[i+2]&0x3f), i+3
		default:
			flush()
			b.WriteRune(utf8.RuneError)
			i++
			continue
		}
		switch {
		case utf16.IsSurrogate(r) && r < 0xdc00:
			flush()
			pending = r
		case utf16.IsSurrogate(r):
			if pending != 0 {
				b.WriteRune(utf16.DecodeRune(pending, r))
				pending = 0
			} else {
				b.WriteRune(utf8.RuneError)
			}
		default:
			flush()
			b.WriteRune(r)
		}
	}
	flush()
	return b.String()
}

// isPlain reports whether s is ASCII without NUL, where both encodings agree.
func isPlain(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 || s[i] >= 0x80 {
			return false
		}
	}
	return true
}
