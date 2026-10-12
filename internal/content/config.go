package content

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"quartz/internal/core/fsx"
)

// Two small game files hold content state the game owns: options.txt
// (resource pack order) and Iris/Oculus' properties (the active shader).
// Both are edited key by key; every other line stays byte for byte.

const maxConfig = 1 << 20

// lines is a config file split into lines, without line endings.
type lines struct {
	text  []string
	sep   string // the key/value separator: ":" or "="
	crlf  bool
	exist bool
}

func readLines(path, sep string) (lines, error) {
	data, err := fsx.ReadCapped(path, maxConfig)
	if errors.Is(err, fs.ErrNotExist) {
		return lines{sep: sep}, nil
	}
	if err != nil {
		return lines{}, err
	}
	l := lines{sep: sep, exist: true, crlf: bytes.Contains(data, []byte("\r\n"))}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text != "" {
		l.text = strings.Split(text, "\n")
	}
	return l, nil
}

func (l lines) write(path string) error {
	nl := "\n"
	if l.crlf {
		nl = "\r\n"
	}
	data := strings.Join(l.text, nl) + nl
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fsx.AtomicWrite(path, []byte(data), fsx.FilePerm)
}

// key returns the key of a line ("" for comments and blank lines).
func (l lines) key(line string) string {
	if strings.HasPrefix(strings.TrimSpace(line), "#") || strings.HasPrefix(strings.TrimSpace(line), "!") {
		return ""
	}
	if l.sep == "=" {
		i := strings.IndexAny(line, "=:")
		if i < 0 {
			return ""
		}
		return strings.TrimSpace(line[:i])
	}
	k, _, ok := strings.Cut(line, l.sep)
	if !ok {
		return ""
	}
	return k
}

func (l lines) get(key string) (string, bool) {
	for _, line := range l.text {
		if l.key(line) == key {
			v := line[strings.IndexAny(line, l.sep+"=:")+1:]
			if l.sep == "=" {
				return unescapeProp(strings.TrimLeft(v, " \t")), true
			}
			return v, true
		}
	}
	return "", false
}

// set replaces a key's line, or appends one.
func (l *lines) set(key, value string) {
	if l.sep == "=" {
		value = escapeProp(value)
	}
	line := key + l.sep + value
	for i, old := range l.text {
		if l.key(old) == key {
			l.text[i] = line
			return
		}
	}
	l.text = append(l.text, line)
}

// ---- options.txt ----

// optionsPath is the game's options file.
func optionsPath(game string) string { return filepath.Join(game, "options.txt") }

// packID is how options.txt names a pack in resourcepacks/.
func packID(name string) string { return "file/" + name }

func readPackList(l lines, key string) []string {
	v, ok := l.get(key)
	if !ok {
		return nil
	}
	var out []string
	if json.Unmarshal([]byte(v), &out) != nil {
		return nil
	}
	return out
}

func encodePackList(ids []string) string {
	if ids == nil {
		ids = []string{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

// orderPacks rewrites the resource pack list: built-in packs (vanilla,
// fabric, …) keep their place at the bottom, then the wanted packs, given
// top (highest priority) first, as the game stores them: lowest first.
// Packs the game would call incompatible are also listed in
// incompatibleResourcePacks, or it would drop them on start.
func orderPacks(l *lines, wanted []string, incompatible map[string]bool, worldVersion int) {
	var list []string
	for _, id := range readPackList(*l, "resourcePacks") {
		if !strings.HasPrefix(id, "file/") {
			list = append(list, id)
		}
	}
	if !l.exist && len(list) == 0 {
		list = []string{"vanilla"}
	}
	var incompat []string
	for i := len(wanted) - 1; i >= 0; i-- {
		list = append(list, packID(wanted[i]))
		if incompatible[wanted[i]] {
			incompat = append(incompat, packID(wanted[i]))
		}
	}
	if !l.exist && worldVersion > 0 {
		l.set("version", strconv.Itoa(worldVersion))
	}
	l.set("resourcePacks", encodePackList(list))
	l.set("incompatibleResourcePacks", encodePackList(incompat))
}

// enabledPacks lists the enabled packs in resourcepacks/, top first.
func enabledPacks(l lines) []string {
	list := readPackList(l, "resourcePacks")
	var out []string
	for i := len(list) - 1; i >= 0; i-- {
		if name, ok := strings.CutPrefix(list[i], "file/"); ok {
			out = append(out, name)
		}
	}
	return out
}

// ---- Iris / Oculus properties ----

// escapeProp escapes a Java properties value (ISO-8859-1 with \u escapes).
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

// unescapeProp reverses escapeProp (and Java's other escapes).
func unescapeProp(v string) string {
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

// shaderConfig is the properties file of the shader mod an instance has.
func shaderConfig(game string, oculus bool) string {
	if oculus {
		return filepath.Join(game, "config", "oculus.properties")
	}
	return filepath.Join(game, "config", "iris.properties")
}

// activeShader reads the selected shader pack ("" when shaders are off).
func activeShader(l lines) string {
	if v, _ := l.get("enableShaders"); strings.EqualFold(strings.TrimSpace(v), "false") {
		return ""
	}
	v, _ := l.get("shaderPack")
	return v
}

func setShader(l *lines, name string) {
	if name == "" {
		l.set("enableShaders", "false")
		return
	}
	l.set("shaderPack", name)
	l.set("enableShaders", "true")
}
