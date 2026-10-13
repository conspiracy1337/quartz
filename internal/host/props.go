package host

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"unicode"

	"quartz/internal/core/fsx"
)

// maxProps caps server.properties when it is read back.
const maxProps = 256 << 10

// writeProps sets keys in a Java properties file and keeps everything else,
// comments and the player's own edits included. A key Quartz sets replaces
// its line where it is; new keys go at the end.
func writeProps(path string, set map[string]string, keepIfSet map[string]string) error {
	raw, err := fsx.ReadCapped(path, maxProps)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	seen := map[string]bool{}
	for i, l := range lines {
		k := propKey(l)
		if k == "" {
			continue
		}
		seen[k] = true
		if v, ok := set[k]; ok {
			lines[i] = k + "=" + escapeProp(v)
		}
	}
	for _, m := range []map[string]string{set, keepIfSet} {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if !seen[k] {
				lines = append(lines, k+"="+escapeProp(m[k]))
				seen[k] = true
			}
		}
	}
	return fsx.AtomicWrite(path, []byte(strings.Join(lines, "\n")+"\n"), fsx.FilePerm)
}

// propKey is the key a properties line sets ("" for comments and blanks).
func propKey(line string) string {
	t := strings.TrimLeft(line, " \t\f")
	if t == "" || t[0] == '#' || t[0] == '!' {
		return ""
	}
	end := strings.IndexAny(t, "=: \t\f")
	if end < 0 {
		return t
	}
	return t[:end]
}

// escapeProp writes a value the way java.util.Properties reads it back on
// every Minecraft version: backslashes doubled, a leading space kept, and
// anything outside printable ASCII as \uXXXX. Line breaks never reach here
// (values are checked), but are escaped all the same.
func escapeProp(v string) string {
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
		case r < 0x20 || r > 0x7e:
			for _, u := range utf16Of(r) {
				fmt.Fprintf(&b, `\u%04x`, u)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func utf16Of(r rune) []uint16 {
	if r < 0x10000 {
		return []uint16{uint16(r)} //nolint:gosec // below 0x10000
	}
	r -= 0x10000
	return []uint16{uint16(0xd800 + (r>>10)&0x3ff), uint16(0xdc00 + r&0x3ff)} //nolint:gosec // 10-bit halves
}

// motd is the server's description: the instance's name, printable and short.
func motd(name string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) && r != '§' && r != '[' && r != ']' {
			return r
		}
		return -1
	}, name)
	if r := []rune(s); len(r) > 59 {
		s = string(r[:59])
	}
	return strings.TrimSpace(s)
}
