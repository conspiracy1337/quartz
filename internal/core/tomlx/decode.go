// Package tomlx reads TOML documents into map[string]any trees and writes them
// back, as github.com/pelletier/go-toml/v2's Unmarshal and Marshal do for that
// type. It runs on go-toml's own parser (the unstable package) with copies of
// the few pieces of go-toml it needs (the duplicate-key tracker, value
// conversion, the encoder's layout rules), leaving out go-toml's reflection
// codec, which the launcher never uses.
//
// Parts are copied from go-toml v2: Copyright (c) 2021 - 2023 Thomas
// Pelletier, Eric Anderton; MIT License,
// https://github.com/pelletier/go-toml/blob/v2/LICENSE.
package tomlx

import (
	"bytes"
	"math"
	"strconv"
	"time"

	"github.com/pelletier/go-toml/v2/unstable"
)

// Decode parses a document. Values are string, int64, float64, bool,
// time.Time (offset date-times), LocalDate, LocalTime, LocalDateTime, []any
// and map[string]any. Arrays of tables are []any of map[string]any.
func Decode(data []byte) (map[string]any, error) {
	var p unstable.Parser
	var seen seenTracker
	root := map[string]any{}
	cur := root
	p.Reset(data)
	for p.NextExpression() {
		expr := p.Expression()
		first, err := seen.checkExpression(expr)
		if err != nil {
			return nil, err
		}
		switch expr.Kind {
		case unstable.Table:
			cur = root
			for it := expr.Key(); it.Next(); {
				cur = childTable(cur, string(it.Node().Data))
			}
		case unstable.ArrayTable:
			cur = arrayTableElem(root, expr.Key(), first)
		case unstable.KeyValue:
			if err := setKey(cur, expr.Key(), expr.Value()); err != nil {
				return nil, err
			}
		}
	}
	if err := p.Error(); err != nil {
		return nil, err
	}
	return root, nil
}

// arrayTableElem appends a new table to the [[array]] named by key and
// returns it. first is true the first time the header appears.
func arrayTableElem(root map[string]any, key unstable.Iterator, first bool) map[string]any {
	cur := root
	key.Next()
	name := string(key.Node().Data)
	for key.Next() {
		cur = childTable(cur, name)
		name = string(key.Node().Data)
	}
	s, _ := cur[name].([]any)
	if first {
		s = s[:0]
	}
	elem := map[string]any{}
	cur[name] = append(s, elem)
	return elem
}

// setKey stores value at the (possibly dotted) key inside cur.
func setKey(cur map[string]any, key unstable.Iterator, value *unstable.Node) error {
	for key.Next() {
		name := string(key.Node().Data)
		if key.IsLast() {
			v, err := decodeValue(value)
			if err != nil {
				return err
			}
			cur[name] = v
			return nil
		}
		cur = childTable(cur, name)
	}
	return nil
}

// childTable returns the table at name in cur, creating it when absent and
// descending into the last element of an array of tables. The tracker has
// already rejected documents where name holds anything else.
func childTable(cur map[string]any, name string) map[string]any {
	switch v := cur[name].(type) {
	case map[string]any:
		return v
	case []any:
		if len(v) > 0 {
			if last, ok := v[len(v)-1].(map[string]any); ok {
				return last
			}
		}
	}
	m := map[string]any{}
	cur[name] = m
	return m
}

func decodeValue(n *unstable.Node) (any, error) {
	switch n.Kind {
	case unstable.String:
		return string(n.Data), nil
	case unstable.Integer:
		return parseInteger(n.Data)
	case unstable.Float:
		return parseFloat(n.Data)
	case unstable.Bool:
		return n.Data[0] == 't', nil
	case unstable.Array:
		var out []any
		for it := n.Children(); it.Next(); {
			c := it.Node()
			if c.Kind == unstable.Comment {
				continue
			}
			v, err := decodeValue(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if out == nil {
			out = []any{}
		}
		return out, nil
	case unstable.InlineTable:
		m := map[string]any{}
		for it := n.Children(); it.Next(); {
			kv := it.Node()
			if err := setKey(m, kv.Key(), kv.Value()); err != nil {
				return nil, err
			}
		}
		return m, nil
	case unstable.DateTime:
		return parseDateTime(n.Data)
	case unstable.LocalDateTime:
		dt, rest, err := parseLocalDateTime(n.Data)
		if err != nil {
			return nil, err
		}
		if len(rest) > 0 {
			return nil, unstable.NewParserError(rest, "extra characters at the end of a local date time")
		}
		return dt, nil
	case unstable.LocalDate:
		return parseLocalDate(n.Data)
	case unstable.LocalTime:
		t, rest, err := parseLocalTime(n.Data)
		if err != nil {
			return nil, err
		}
		if len(rest) > 0 {
			return nil, unstable.NewParserError(rest, "extra characters at the end of a local time")
		}
		return t, nil
	}
	return nil, unstable.NewParserError(n.Data, "unsupported value kind %s", n.Kind)
}

// parseInteger reads an integer the parser has already checked the syntax of.
func parseInteger(b []byte) (int64, error) {
	if len(b) > 2 && b[0] == '0' {
		switch b[1] {
		case 'x':
			return parseBase(b, 4, "hexadecimal")
		case 'b':
			return parseBase(b, 1, "binary")
		case 'o':
			return parseBase(b, 3, "octal")
		}
	}
	return parseIntDec(b)
}

// parseBase reads 0x, 0o or 0b digits, bits per digit at a time.
func parseBase(b []byte, bits uint, name string) (int64, error) {
	var v uint64
	for _, c := range b[2:] {
		if c == '_' {
			continue
		}
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		}
		if v > math.MaxInt64>>bits {
			return 0, unstable.NewParserError(b, "%s number is too large to fit in a 64-bit signed integer", name)
		}
		v = v<<bits | uint64(d)
	}
	return int64(v), nil //nolint:gosec // checked against MaxInt64 above
}

func parseIntDec(b []byte) (int64, error) {
	i, neg := 0, false
	switch b[0] {
	case '-':
		neg = true
		i++
	case '+':
		i++
	}
	var limit uint64 = math.MaxInt64
	if neg {
		limit = math.MaxInt64 + 1
	}
	var v uint64
	for ; i < len(b); i++ {
		c := b[i]
		if c == '_' {
			continue
		}
		if v > limit/10 {
			return 0, unstable.NewParserError(b, "decimal number is too large to fit in a 64-bit signed integer")
		}
		v = v*10 + uint64(c-'0')
		if v > limit {
			return 0, unstable.NewParserError(b, "decimal number is too large to fit in a 64-bit signed integer")
		}
	}
	if neg {
		return -int64(v), nil //nolint:gosec // v <= MaxInt64+1 wraps to the intended negative value
	}
	return int64(v), nil //nolint:gosec // v <= MaxInt64
}

// parseFloat reads a float the parser has already checked the syntax of.
// go-toml tries an exact fast path first; strconv rounds the same way.
func parseFloat(b []byte) (float64, error) {
	i := 0
	if len(b) > 0 && (b[0] == '+' || b[0] == '-') {
		i = 1
	}
	if len(b) == i+3 {
		switch b[i] {
		case 'i':
			if b[0] == '-' {
				return math.Inf(-1), nil
			}
			return math.Inf(1), nil
		case 'n':
			return math.NaN(), nil
		}
	}
	cleaned := b
	if bytes.IndexByte(b, '_') >= 0 {
		cleaned = bytes.ReplaceAll(b, []byte{'_'}, nil)
	}
	f, err := strconv.ParseFloat(string(cleaned), 64)
	if err != nil {
		return 0, unstable.NewParserError(b, "unable to parse float: %s", err)
	}
	return f, nil
}

func parseLocalDate(b []byte) (LocalDate, error) {
	var date LocalDate
	if len(b) != 10 || b[4] != '-' || b[7] != '-' {
		return date, unstable.NewParserError(b, "dates are expected to have the format YYYY-MM-DD")
	}
	var err error
	if date.Year, err = parseDecimalDigits(b[0:4]); err != nil {
		return date, err
	}
	if date.Month, err = parseDecimalDigits(b[5:7]); err != nil {
		return date, err
	}
	if date.Day, err = parseDecimalDigits(b[8:10]); err != nil {
		return date, err
	}
	if date.Month < 1 || date.Month > 12 {
		return date, unstable.NewParserError(b[5:7], "impossible date")
	}
	if date.Day < 1 || date.Day > daysIn(date.Month, date.Year) {
		return date, unstable.NewParserError(b[8:10], "impossible date")
	}
	return date, nil
}

func daysIn(month, year int) int {
	switch month {
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

func parseDecimalDigits(b []byte) (int, error) {
	v := 0
	for i, c := range b {
		if c < '0' || c > '9' {
			return 0, unstable.NewParserError(b[i:i+1], "expected digit (0-9)")
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}

// parseLocalTime reads HH:MM[:SS[.fraction]] and returns what follows.
func parseLocalTime(b []byte) (LocalTime, []byte, error) {
	nspow := [10]int{0, 1e8, 1e7, 1e6, 1e5, 1e4, 1e3, 1e2, 1e1, 1e0}
	var t LocalTime
	if len(b) < 5 {
		return t, nil, unstable.NewParserError(b, "times are expected to have the format HH:MM[:SS[.NNNNNN]]")
	}
	var err error
	if t.Hour, err = parseDecimalDigits(b[0:2]); err != nil {
		return t, nil, err
	}
	if t.Hour > 23 {
		return t, nil, unstable.NewParserError(b[0:2], "hour cannot be greater 23")
	}
	if b[2] != ':' {
		return t, nil, unstable.NewParserError(b[2:3], "expecting colon between hours and minutes")
	}
	if t.Minute, err = parseDecimalDigits(b[3:5]); err != nil {
		return t, nil, err
	}
	if t.Minute > 59 {
		return t, nil, unstable.NewParserError(b[3:5], "minutes cannot be greater 59")
	}
	b = b[5:]
	// Seconds are optional (TOML 1.1); a fraction needs them.
	if len(b) == 0 || b[0] != ':' {
		return t, b, nil
	}
	if len(b) < 3 {
		return t, nil, unstable.NewParserError(b, "incomplete seconds")
	}
	if t.Second, err = parseDecimalDigits(b[1:3]); err != nil {
		return t, nil, err
	}
	if t.Second > 59 {
		return t, nil, unstable.NewParserError(b[1:3], "seconds cannot be greater than 59")
	}
	b = b[3:]
	if len(b) == 0 || b[0] != '.' {
		return t, b, nil
	}
	frac, precision, digits := 0, 0, 0
	for i, c := range b[1:] {
		if c < '0' || c > '9' {
			if i == 0 {
				return t, nil, unstable.NewParserError(b[0:1], "need at least one digit after fraction point")
			}
			break
		}
		digits++
		if i < 9 {
			frac = frac*10 + int(c-'0')
			precision++
		}
	}
	if digits == 0 {
		return t, nil, unstable.NewParserError(b[0:1], "need at least one digit after fraction point")
	}
	t.Nanosecond = frac * nspow[precision]
	t.Precision = precision
	return t, b[1+digits:], nil
}

// parseLocalDateTime reads YYYY-MM-DD(T|t| )time and returns what follows.
func parseLocalDateTime(b []byte) (LocalDateTime, []byte, error) {
	var dt LocalDateTime
	if len(b) < 11 {
		return dt, nil, unstable.NewParserError(b, "local datetimes are expected to have the format YYYY-MM-DDTHH:MM[:SS[.NNNNNNNNN]]")
	}
	date, err := parseLocalDate(b[:10])
	if err != nil {
		return dt, nil, err
	}
	dt.LocalDate = date
	if sep := b[10]; sep != 'T' && sep != ' ' && sep != 't' {
		return dt, nil, unstable.NewParserError(b[10:11], "datetime separator is expected to be T or a space")
	}
	t, rest, err := parseLocalTime(b[11:])
	if err != nil {
		return dt, nil, err
	}
	dt.LocalTime = t
	return dt, rest, nil
}

// parseDateTime reads a date-time with a Z or ±HH:MM offset.
func parseDateTime(b []byte) (time.Time, error) {
	dt, b, err := parseLocalDateTime(b)
	if err != nil {
		return time.Time{}, err
	}
	if len(b) == 0 {
		return time.Time{}, unstable.NewParserError(b, "date-time is missing timezone")
	}
	var zone *time.Location
	if b[0] == 'Z' || b[0] == 'z' {
		b = b[1:]
		zone = time.UTC
	} else {
		if len(b) != 6 {
			return time.Time{}, unstable.NewParserError(b, "invalid date-time timezone")
		}
		var direction int
		switch b[0] {
		case '-':
			direction = -1
		case '+':
			direction = +1
		default:
			return time.Time{}, unstable.NewParserError(b[:1], "invalid timezone offset character")
		}
		if b[3] != ':' {
			return time.Time{}, unstable.NewParserError(b[3:4], "expected a : separator")
		}
		hours, err := parseDecimalDigits(b[1:3])
		if err != nil {
			return time.Time{}, err
		}
		if hours > 23 {
			return time.Time{}, unstable.NewParserError(b[1:3], "invalid timezone offset hours")
		}
		minutes, err := parseDecimalDigits(b[4:6])
		if err != nil {
			return time.Time{}, err
		}
		if minutes > 59 {
			return time.Time{}, unstable.NewParserError(b[4:6], "invalid timezone offset minutes")
		}
		if seconds := direction * (hours*3600 + minutes*60); seconds == 0 {
			zone = time.UTC
		} else {
			zone = time.FixedZone("", seconds)
		}
		b = b[6:]
	}
	if len(b) > 0 {
		return time.Time{}, unstable.NewParserError(b, "extra bytes at the end of the timezone")
	}
	return time.Date(dt.Year, time.Month(dt.Month), dt.Day, dt.Hour, dt.Minute, dt.Second, dt.Nanosecond, zone), nil
}
