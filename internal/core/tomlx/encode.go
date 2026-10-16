package tomlx

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Encode writes doc the way go-toml's Marshal does with its defaults: keys
// sorted, plain values before tables, sub-tables as [a.b] sections, non-empty
// lists of tables as [[a]] sections, everything else inline. It takes the
// types Decode returns plus int, []string and []map[string]any.
func Encode(doc map[string]any) ([]byte, error) {
	var e encoder
	if err := e.table(doc); err != nil {
		return nil, err
	}
	return e.buf, nil
}

type encoder struct {
	buf           []byte
	keys          []string // the dotted key of the table being written
	lastWasHeader bool     // no blank line between consecutive headers
}

type entry struct {
	key   string
	value any
}

// entries lists m by key, skipping nil values as go-toml does.
func entries(m map[string]any) []entry {
	out := make([]entry, 0, len(m))
	for k, v := range m {
		if v != nil {
			out = append(out, entry{k, v})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

func isTable(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

// isArrayOfTables reports a non-empty list holding only tables.
func isArrayOfTables(v any) bool {
	switch l := v.(type) {
	case []map[string]any:
		return len(l) > 0
	case []any:
		for _, x := range l {
			if !isTable(x) {
				return false
			}
		}
		return len(l) > 0
	}
	return false
}

func (e *encoder) table(m map[string]any) error {
	list := entries(m)
	for _, ent := range list {
		if isTable(ent.value) || isArrayOfTables(ent.value) {
			continue
		}
		if err := e.keyValue(ent); err != nil {
			return err
		}
	}
	for _, ent := range list {
		e.keys = append(e.keys, ent.key)
		switch v := ent.value.(type) {
		case map[string]any:
			e.header(false)
			if err := e.table(v); err != nil {
				return err
			}
		case []map[string]any:
			for _, t := range v {
				e.header(true)
				if err := e.table(t); err != nil {
					return err
				}
			}
		case []any:
			if isArrayOfTables(v) {
				for _, t := range v {
					e.header(true)
					if err := e.table(t.(map[string]any)); err != nil {
						return err
					}
				}
			}
		}
		e.keys = e.keys[:len(e.keys)-1]
	}
	return nil
}

func (e *encoder) header(array bool) {
	if len(e.buf) > 0 && !e.lastWasHeader {
		e.buf = append(e.buf, '\n')
	}
	e.buf = append(e.buf, '[')
	if array {
		e.buf = append(e.buf, '[')
	}
	for i, k := range e.keys {
		if i > 0 {
			e.buf = append(e.buf, '.')
		}
		e.buf = appendKey(e.buf, k)
	}
	e.buf = append(e.buf, ']')
	if array {
		e.buf = append(e.buf, ']')
	}
	e.buf = append(e.buf, '\n')
	e.lastWasHeader = true
}

func (e *encoder) keyValue(ent entry) error {
	e.buf = appendKey(e.buf, ent.key)
	e.buf = append(e.buf, " = "...)
	var err error
	if e.buf, err = appendValue(e.buf, ent.value); err != nil {
		return err
	}
	e.buf = append(e.buf, '\n')
	e.lastWasHeader = false
	return nil
}

func appendValue(b []byte, v any) ([]byte, error) {
	switch x := v.(type) {
	case time.Time:
		return x.AppendFormat(b, "2006-01-02T15:04:05.999999999Z07:00"), nil
	case LocalDate:
		return append(b, x.String()...), nil
	case LocalTime:
		return append(b, x.String()...), nil
	case LocalDateTime:
		return append(b, x.String()...), nil
	case string:
		return appendString(b, x), nil
	case bool:
		return strconv.AppendBool(b, x), nil
	case int:
		return strconv.AppendInt(b, int64(x), 10), nil
	case int64:
		return strconv.AppendInt(b, x, 10), nil
	case float64:
		return appendFloat(b, x), nil
	case []string:
		return appendArray(b, len(x), func(b []byte, i int) ([]byte, error) { return appendString(b, x[i]), nil })
	case []any:
		return appendArray(b, len(x), func(b []byte, i int) ([]byte, error) {
			if x[i] == nil {
				return nil, fmt.Errorf("toml: cannot encode a nil interface")
			}
			return appendValue(b, x[i])
		})
	case []map[string]any:
		return appendArray(b, len(x), func(b []byte, i int) ([]byte, error) { return appendInlineTable(b, x[i]) })
	case map[string]any:
		return appendInlineTable(b, x)
	}
	return nil, fmt.Errorf("toml: cannot encode value of type %T", v)
}

func appendArray(b []byte, n int, elem func([]byte, int) ([]byte, error)) ([]byte, error) {
	b = append(b, '[')
	for i := range n {
		if i > 0 {
			b = append(b, ", "...)
		}
		var err error
		if b, err = elem(b, i); err != nil {
			return nil, err
		}
	}
	return append(b, ']'), nil
}

func appendInlineTable(b []byte, m map[string]any) ([]byte, error) {
	b = append(b, '{')
	for i, ent := range entries(m) {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = appendKey(b, ent.key)
		b = append(b, " = "...)
		var err error
		if b, err = appendValue(b, ent.value); err != nil {
			return nil, err
		}
	}
	return append(b, '}'), nil
}

func appendFloat(b []byte, f float64) []byte {
	switch {
	case math.IsNaN(f):
		return append(b, "nan"...)
	case math.IsInf(f, 1):
		return append(b, "inf"...)
	case math.IsInf(f, -1):
		return append(b, "-inf"...)
	}
	start := len(b)
	b = strconv.AppendFloat(b, f, 'f', -1, 64)
	// TOML floats need a fraction or an exponent.
	if !strings.ContainsAny(string(b[start:]), ".eE") {
		b = append(b, ".0"...)
	}
	return b
}

func appendKey(b []byte, key string) []byte {
	if key == "" {
		return appendString(b, key)
	}
	for i := range len(key) {
		if c := key[i]; !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return appendString(b, key)
		}
	}
	return append(b, key...)
}

// appendString writes a literal ('…') string when s allows one, else a basic one.
func appendString(b []byte, s string) []byte {
	if canBeLiteral(s) {
		b = append(b, '\'')
		b = append(b, s...)
		return append(b, '\'')
	}
	return appendBasicString(b, s)
}

func canBeLiteral(s string) bool {
	for i := range len(s) {
		if c := s[i]; c == '\'' || c == 0x7f || c < 0x20 {
			return false
		}
	}
	return utf8.ValidString(s)
}

func appendBasicString(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			b = append(b, '\\', '"')
		case c == '\\':
			b = append(b, '\\', '\\')
		case c == '\b':
			b = append(b, '\\', 'b')
		case c == '\f':
			b = append(b, '\\', 'f')
		case c == '\n':
			b = append(b, '\\', 'n')
		case c == '\r':
			b = append(b, '\\', 'r')
		case c == '\t':
			b = append(b, '\\', 't')
		case c < 0x20 || c == 0x7f:
			b = fmt.Appendf(b, "\\u%04X", c)
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				// go-toml writes an invalid byte as its \u escape.
				b = fmt.Appendf(b, "\\u%04X", c)
				i++
				continue
			}
			b = append(b, s[i:i+size]...)
			i += size
			continue
		}
		i++
	}
	return append(b, '"')
}
