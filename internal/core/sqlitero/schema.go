package sqlitero

import (
	"strings"
)

// parseColumns reads the column names from a CREATE TABLE statement, and
// which column (if any) is the INTEGER PRIMARY KEY that aliases the rowid
// (its value lives in the rowid, not the record).
func parseColumns(sql string) (cols []string, rowidCol int, err error) {
	rowidCol = -1
	open := strings.IndexByte(sql, '(')
	end := strings.LastIndexByte(sql, ')')
	if open < 0 || end <= open {
		return nil, -1, ErrCorrupt
	}
	for _, def := range splitTop(sql[open+1 : end]) {
		def = strings.TrimSpace(def)
		name, rest := firstToken(def)
		switch strings.ToUpper(name) {
		case "", "CONSTRAINT", "PRIMARY", "UNIQUE", "CHECK", "FOREIGN":
			continue
		}
		up := strings.ToUpper(strings.Join(strings.Fields(rest), " "))
		if strings.HasPrefix(up, "INTEGER PRIMARY KEY") && !strings.Contains(up, "DESC") {
			rowidCol = len(cols)
		}
		cols = append(cols, name)
	}
	if len(cols) == 0 {
		return nil, -1, ErrCorrupt
	}
	return cols, rowidCol, nil
}

// splitTop splits on commas outside parentheses and quotes.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '[':
			quote = ']'
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// firstToken returns a column definition's (unquoted) name and the rest.
func firstToken(def string) (string, string) {
	if def == "" {
		return "", ""
	}
	closer := map[byte]byte{'"': '"', '`': '`', '[': ']'}
	if c, ok := closer[def[0]]; ok {
		if end := strings.IndexByte(def[1:], c); end >= 0 {
			return def[1 : end+1], def[end+2:]
		}
		return "", ""
	}
	i := strings.IndexFunc(def, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	if i < 0 {
		return def, ""
	}
	return def[:i], def[i:]
}
