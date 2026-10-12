package meta

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Lengths text from a file is cut to, so one silly file can't bloat the UI.
const (
	maxName    = 128
	maxDesc    = 512
	maxAuthors = 8
	maxID      = 64
	maxDeps    = 128
)

// clean strips formatting codes and control characters, collapses
// whitespace and truncates to n runes.
func clean(s string, n int) string {
	var b strings.Builder
	space := false
	skip := false
	for _, r := range s {
		switch {
		case skip:
			skip = false
			continue
		case r == '§':
			skip = true
			continue
		case r == utf8.RuneError || r < 0x20 || r == 0x7f || r == ' ':
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	out := b.String()
	if utf8.RuneCountInString(out) > n {
		out = string([]rune(out)[:n-1]) + "…"
	}
	return out
}

func cleanList(in []string, n int) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = clean(s, maxName)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) == n {
			break
		}
	}
	return out
}

// lenientJSON fixes what mod authors commonly get wrong in JSON files that
// Java's lenient parsers accept: a byte-order mark, and raw newlines or tabs
// inside strings.
func lenientJSON(b []byte) []byte {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var out []byte
	in, esc := false, false
	for i, c := range b {
		if in && c < 0x20 {
			if out == nil {
				out = append(make([]byte, 0, len(b)+16), b[:i]...)
			}
			out = append(out, ' ')
			continue
		}
		switch {
		case esc:
			esc = false
		case c == '\\' && in:
			esc = true
		case c == '"':
			in = !in
		}
		if out != nil {
			out = append(out, c)
		}
	}
	if out == nil {
		return b
	}
	return out
}

func decodeJSON(b []byte, v any) error { return json.Unmarshal(lenientJSON(b), v) }

// component flattens a Minecraft text component (a string, an object with
// text and extra, or a list of them) to plain text.
func component(raw json.RawMessage) string {
	var b strings.Builder
	flatten(raw, &b, 0)
	return b.String()
}

func flatten(raw json.RawMessage, b *strings.Builder, depth int) {
	if depth > 8 || len(raw) == 0 || b.Len() > maxDesc*4 {
		return
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		b.WriteString(s)
		return
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		for _, e := range list {
			flatten(e, b, depth+1)
		}
		return
	}
	var obj struct {
		Text      json.RawMessage   `json:"text"`
		Translate string            `json:"translate"`
		Extra     []json.RawMessage `json:"extra"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if len(obj.Text) > 0 {
			flatten(obj.Text, b, depth+1)
		} else {
			b.WriteString(obj.Translate)
		}
		for _, e := range obj.Extra {
			flatten(e, b, depth+1)
		}
		return
	}
	// Numbers and booleans are valid components too.
	b.Write(bytes.Trim(raw, `"`))
}
