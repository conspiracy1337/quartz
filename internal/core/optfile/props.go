package optfile

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

// EscapeProp escapes a Java properties value (ISO-8859-1 with \u escapes).
func EscapeProp(v string) string {
	var b strings.Builder
	for i, r := range v {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == ' ' && i == 0:
			b.WriteString(`\ `)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r > 0x7e:
			if r > 0xffff {
				for _, c := range utf16Pair(r) {
					fmt.Fprintf(&b, `\u%04x`, c)
				}
				continue
			}
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func utf16Pair(r rune) [2]rune {
	r -= 0x10000
	return [2]rune{0xd800 + (r>>10)&0x3ff, 0xdc00 + r&0x3ff}
}

// UnescapeProp reverses EscapeProp (and Java's other escapes).
func UnescapeProp(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var out []rune
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c != '\\' || i+1 == len(v) {
			r, n := utf8.DecodeRuneInString(v[i:])
			out = append(out, r)
			i += n - 1
			continue
		}
		i++
		switch v[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			if i+4 < len(v) {
				if b, err := hex.DecodeString(v[i+1 : i+5]); err == nil {
					out = append(out, rune(b[0])<<8|rune(b[1]))
					i += 4
					continue
				}
			}
			out = append(out, 'u')
		default:
			out = append(out, rune(v[i]))
		}
	}
	// Join surrogate pairs written as two \u escapes.
	return string(utf16Decode(out))
}

func utf16Decode(rs []rune) []rune {
	var out []rune
	for i := 0; i < len(rs); i++ {
		if rs[i] >= 0xd800 && rs[i] < 0xdc00 && i+1 < len(rs) && rs[i+1] >= 0xdc00 && rs[i+1] < 0xe000 {
			out = append(out, 0x10000+(rs[i]-0xd800)<<10+(rs[i+1]-0xdc00))
			i++
			continue
		}
		out = append(out, rs[i])
	}
	return out
}
