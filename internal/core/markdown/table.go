package markdown

import "strings"

// GFM tables, as goldmark finds them: when a paragraph closes, its first
// line that reads as a delimiter row ("| --- | :-: |") makes the line above
// it the header and every later line a row. Lines before the header stay
// a paragraph.

// tableAt is the index of the header line of the table in a paragraph's
// lines, or -1.
func tableAt(content string, indents []int) int {
	lines := strings.SplitAfter(content, "\n")
	for i := 1; i < len(lines); i++ {
		indent := 0
		if i < len(indents) {
			indent = indents[i]
		}
		if indent > 3 {
			continue
		}
		aligns := delimiterRow(lines[i], indent > 0)
		if aligns == nil {
			continue
		}
		if len(tableRow(lines[i-1], aligns, true)) != len(aligns) {
			return -1
		}
		return i - 1
	}
	return -1
}

// table turns the paragraph's lines into a table after it, if they hold
// one, and returns what stays paragraph.
func (p *parser) table(n *node, content string, indents []int) string {
	h := tableAt(content, indents)
	if h < 0 {
		return content
	}
	lines := strings.SplitAfter(content, "\n")
	aligns := delimiterRow(lines[h+1], h+1 < len(indents) && indents[h+1] > 0)
	t := &node{kind: kTable, startLine: n.startLine + h, endLine: n.endLine}
	for i, line := range lines[h:] {
		if i == 1 || line == "" {
			continue
		}
		row := &node{kind: kTableRow, header: i == 0}
		for j, cell := range tableRow(line, aligns, i == 0) {
			c := &node{kind: kTableCell, content: []byte(cell), align: aligns[j]}
			row.appendChild(c)
		}
		t.appendChild(row)
	}
	n.insertAfter(t)
	n.before = n.before[:min(h, len(n.before))]
	n.indents = n.indents[:min(h, len(n.indents))]
	return strings.TrimSuffix(strings.Join(lines[:h], ""), "\n")
}

// delimiterRow reads a delimiter row's column alignments ('l', 'c', 'r'
// or 0), or nil when the line isn't one. indented tells that the line had
// leading space before it was trimmed: goldmark refuses a line of dashes
// only, and a space or line end is something else.
func delimiterRow(line string, indented bool) []byte {
	for i := 0; i < len(line); i++ {
		if c := line[i]; !isSpace(c) && c != '-' && c != '|' && c != ':' {
			return nil
		}
	}
	if !indented && strings.Trim(line, "-") == "" {
		return nil
	}
	cols := strings.Split(line, "|")
	if isBlank(cols[0]) {
		cols = cols[1:]
	}
	if len(cols) > 0 && isBlank(cols[len(cols)-1]) {
		cols = cols[:len(cols)-1]
	}
	var aligns []byte
	for _, col := range cols {
		c := trimSpace(col)
		left, right := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":") && len(c) > 1
		dashes := c
		if left {
			dashes = dashes[1:]
		}
		if right {
			dashes = dashes[:len(dashes)-1]
		}
		if dashes == "" || strings.Trim(dashes, "-") != "" {
			return nil
		}
		switch {
		case left && right:
			aligns = append(aligns, 'c')
		case left:
			aligns = append(aligns, 'l')
		case right:
			aligns = append(aligns, 'r')
		default:
			aligns = append(aligns, 0)
		}
	}
	return aligns
}

// tableRow splits a row into cells at unescaped pipes. A body row keeps as
// many cells as there are columns, filling missing ones with empty cells;
// the header keeps all it has.
func tableRow(line string, aligns []byte, header bool) []string {
	line = trimSpace(line)
	pos, limit := 0, len(line)
	if limit > 0 && line[0] == '|' {
		pos++
	}
	if limit > 0 && line[limit-1] == '|' {
		limit--
	}
	var cells []string
	for pos < limit {
		if len(cells) >= len(aligns) && !header {
			return cells
		}
		end := pos
		for end < limit && (line[end] != '|' || end > 0 && line[end-1] == '\\') {
			end++
		}
		cells = append(cells, trimSpace(line[pos:end]))
		pos = end + 1
	}
	for len(cells) < len(aligns) {
		cells = append(cells, "")
	}
	return cells
}
