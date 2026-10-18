package markdown

import (
	"regexp"
	"strings"
)

// The block parser follows the CommonMark reference implementation
// (commonmark.js): each line first continues the open containers, then may
// start new blocks, and what is left is text. HTML blocks use goldmark's
// rules, and paragraphs become GFM tables as in goldmark.

const (
	codeIndent = 4
	// maxDepth caps nested containers (block quotes, lists): deeper markers
	// are text, so hostile input can't build an arbitrarily deep tree.
	maxDepth = 64
)

var (
	reATXHeading   = regexp.MustCompile(`^#{1,6}(?:[ \t]+|$)`)
	reCodeFence    = regexp.MustCompile("^`{3,}|^~{3,}")
	reClosingFence = regexp.MustCompile("^(?:`{3,}|~{3,})[ \t]*$")
	reSetext       = regexp.MustCompile(`^(?:=+|-+)[ \t]*$`)
	reThematic     = regexp.MustCompile(`^(?:\*[ \t]*){3,}$|^(?:_[ \t]*){3,}$|^(?:-[ \t]*){3,}$`)
	reBullet       = regexp.MustCompile(`^[*+-]`)
	reOrdered      = regexp.MustCompile(`^(\d{1,9})([.)])`)

	// goldmark's HTML block patterns, applied after the indentation.
	reHTML1Open  = regexp.MustCompile(`(?i)^<(script|pre|style|textarea)(?:\s.*|>.*|/>.*|)$`)
	reHTML1Close = regexp.MustCompile(`(?i)</(?:script|pre|style|textarea)>`)
	reHTML4Open  = regexp.MustCompile(`^<![A-Z]+`)
	reHTML6      = regexp.MustCompile(`^<(?:/[ ]*)?([a-zA-Z]+[a-zA-Z0-9\-]*)(?:[ ].*|>.*|/>.*|)$`)
	reHTML7      = regexp.MustCompile(`^<(/[ ]*)?([a-zA-Z]+[a-zA-Z0-9\-]*)(` + attributePattern + `*)[ ]*(?:>|/>)[ ]*$`)
)

var htmlBlockClose = [...]string{2: "-->", 3: "?>", 4: ">", 5: "]]>"}

var blockTags = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range strings.Fields(`address article aside base basefont blockquote body caption center col colgroup dd details
		dialog dir div dl dt fieldset figcaption figure footer form frame frameset h1 h2 h3 h4 h5 h6 head header hr html
		iframe legend li link main menu menuitem meta nav noframes ol optgroup option p param search section summary
		table tbody td tfoot th thead title tr track ul`) {
		m[t] = true
	}
	return m
}()

type parser struct {
	gfm bool

	doc, tip, oldtip     *node
	line                 string
	lineNumber           int
	offset, column       int
	nextNonspace         int
	nextNonspaceColumn   int
	indent               int
	indented, blank      bool
	partiallyConsumedTab bool
	allClosed            bool
	lastMatched          *node
	depth                int // open containers below the document
	refs                 map[string]linkRef
	lineCount            int
	noFinalNewline       bool // the last line has no line end (goldmark's tables see that)
	lineCR               bool // the line ended with \r\n
	// emptyItemBlank is goldmark's emptyListItemWithBlankLines: once a blank
	// line follows an empty list item, every open list ends at the next line
	// that isn't one of its own items, until a new item starts.
	emptyItemBlank bool
	// lineBlank[k] tells whether the line was blank after its first k open
	// blocks took their markers; prevBlank is that for the line before.
	// goldmark marks a new block as after a blank line by prevBlank at the
	// block's depth, which decides whether a list is tight.
	lineBlank, prevBlank []bool
}

type linkRef struct{ dest, title string }

func (p *parser) parse(src string) *node {
	p.doc = &node{kind: kDocument, open: true, startLine: 1}
	p.tip = p.doc
	p.oldtip = p.doc
	p.refs = map[string]linkRef{}
	lines := strings.Split(src, "\n")
	if strings.HasSuffix(src, "\n") {
		lines = lines[:len(lines)-1]
	} else {
		p.noFinalNewline = true
	}
	p.lineCount = len(lines)
	for _, ln := range lines {
		// Lines may end with \r\n; goldmark sees the \r in one place only.
		p.lineCR = strings.HasSuffix(ln, "\r")
		p.incorporateLine(strings.TrimSuffix(ln, "\r"))
	}
	for p.tip != nil {
		p.finalize(p.tip, len(lines))
	}
	p.inlines(p.doc)
	return p.doc
}

func (p *parser) findNextNonspace() {
	i, cols := p.offset, p.column
	for i < len(p.line) {
		if c := p.line[i]; c == ' ' {
			i++
			cols++
		} else if c == '\t' {
			i++
			cols += 4 - cols%4
		} else {
			break
		}
	}
	p.blank = i == len(p.line)
	p.nextNonspace = i
	p.nextNonspaceColumn = cols
	p.indent = cols - p.column
	p.indented = p.indent >= codeIndent
}

func (p *parser) advanceNextNonspace() {
	p.offset = p.nextNonspace
	p.column = p.nextNonspaceColumn
	p.partiallyConsumedTab = false
}

func (p *parser) advanceOffset(count int, columns bool) {
	for count > 0 && p.offset < len(p.line) {
		if p.line[p.offset] == '\t' {
			toTab := 4 - p.column%4
			if columns {
				p.partiallyConsumedTab = toTab > count
				adv := min(toTab, count)
				p.column += adv
				if !p.partiallyConsumedTab {
					p.offset++
				}
				count -= adv
			} else {
				p.partiallyConsumedTab = false
				p.column += toTab
				p.offset++
				count--
			}
		} else {
			p.partiallyConsumedTab = false
			p.offset++
			p.column++
			count--
		}
	}
}

func (p *parser) peek(i int) byte {
	if i < len(p.line) {
		return p.line[i]
	}
	return 0
}

// addLine appends the rest of the line to the tip's content.
func (p *parser) addLine() {
	if p.partiallyConsumedTab {
		p.offset++
		p.tip.content = append(p.tip.content, strings.Repeat(" ", 4-p.column%4)...)
	}
	if p.tip.kind == kParagraph {
		before := '\n'
		switch {
		case len(p.tip.before) == 0:
		case p.partiallyConsumedTab:
			before = ' '
		case p.offset > 0:
			before = lastRune(p.line[:p.offset])
		}
		p.tip.before = append(p.tip.before, before)
		p.tip.indents = append(p.tip.indents, p.indent)
	}
	p.tip.lastText = p.lineNumber
	p.tip.content = append(p.tip.content, p.line[min(p.offset, len(p.line)):]...)
	p.tip.content = append(p.tip.content, '\n')
}

func canContain(parent, child kind) bool {
	switch parent {
	case kDocument, kBlockQuote, kItem:
		return child != kItem
	case kList:
		return child == kItem
	}
	return false
}

func acceptsLines(k kind) bool { return k == kParagraph || k == kCodeBlock || k == kHTMLBlock }

func isContainer(k kind) bool { return k == kBlockQuote || k == kList || k == kItem }

func (p *parser) addChild(k kind) *node {
	for !canContain(p.tip.kind, k) {
		p.finalize(p.tip, p.lineNumber-1)
	}
	n := &node{kind: k, open: true, startLine: p.lineNumber, blankBefore: p.blankAt(p.tip)}
	p.tip.appendChild(n)
	p.tip = n
	if isContainer(k) {
		p.depth++
	}
	return n
}

func (p *parser) closeUnmatched() {
	if p.allClosed {
		return
	}
	for p.oldtip != p.lastMatched {
		parent := p.oldtip.parent
		p.finalize(p.oldtip, p.lineNumber-1)
		p.oldtip = parent
	}
	p.allClosed = true
}

// continues reports whether an open block continues on this line: 0 yes,
// 1 no, 2 yes and the line is used up.
func (p *parser) continues(n *node) int {
	switch n.kind {
	case kBlockQuote:
		if !p.indented && p.peek(p.nextNonspace) == '>' {
			p.advanceNextNonspace()
			p.advanceOffset(1, false)
			if isSpaceOrTab(p.peek(p.offset)) {
				p.advanceOffset(1, true)
			}
			return 0
		}
		return 1
	case kItem:
		switch {
		case p.blank:
			if n.first == nil {
				p.emptyItemBlank = true
				return 1 // a blank line after an empty item ends it
			}
			if p.lineNumber == p.lineCount && p.noFinalNewline {
				return 2 // goldmark reads past an unended last line here: nothing is left of it
			}
			p.advanceNextNonspace()
		case p.indent >= n.list.offset+n.list.padding:
			p.advanceOffset(n.list.offset+n.list.padding, true)
		default:
			return 1
		}
		return 0
	case kCodeBlock:
		if n.fenced {
			if p.indent <= 3 && p.peek(p.nextNonspace) == n.fenceChar {
				if m := reClosingFence.FindString(p.line[p.nextNonspace:]); m != "" && len(strings.TrimRight(m, " \t")) >= n.fenceLength {
					p.finalize(n, p.lineNumber)
					return 2
				}
			}
			if p.blank && p.indent < n.fenceOffset {
				return 0 // goldmark keeps a short blank line whole
			}
			for i := n.fenceOffset; i > 0 && isSpaceOrTab(p.peek(p.offset)); i-- {
				p.advanceOffset(1, true)
			}
			return 0
		}
		switch {
		case p.indent >= codeIndent:
			p.advanceOffset(codeIndent, true)
		case p.blank:
			p.advanceNextNonspace()
		default:
			return 1
		}
		return 0
	case kHTMLBlock:
		if p.blank && (n.htmlType == 6 || n.htmlType == 7) {
			return 1
		}
		return 0
	case kParagraph:
		if p.blank {
			return 1
		}
		return 0
	case kList:
		if p.blank {
			return 0
		}
		// goldmark ends a list whose last item is empty at a marker of
		// another list (or a thematic break) instead of nesting it.
		if last := n.last; last != nil && last.first == nil && p.indent < codeIndent {
			if data := markerAt(p.line[p.nextNonspace:]); data != nil {
				if !listsMatch(n.list, data) || reThematic.MatchString(p.line[p.nextNonspace:]) {
					return 1
				}
				return 0
			}
		}
		if p.emptyItemBlank && !p.listGoesOn(n) {
			return 1
		}
		return 0
	case kDocument:
		return 0
	}
	return 1 // headings, breaks: one line only
}

// blankAt tells whether the line before was blank at the depth of a block
// opened in parent (the first line counts as after a blank one).
func (p *parser) blankAt(parent *node) bool {
	if len(p.prevBlank) == 0 {
		return true
	}
	d := 0
	for n := parent; n.parent != nil; n = n.parent {
		d++
	}
	return p.prevBlank[min(d, len(p.prevBlank)-1)]
}

func (p *parser) incorporateLine(ln string) {
	p.prevBlank, p.lineBlank = p.lineBlank, p.prevBlank[:0]
	container := p.doc
	p.oldtip = p.tip
	p.offset, p.column = 0, 0
	p.blank = false
	p.partiallyConsumedTab = false
	p.lineNumber++
	p.line = ln // NUL stays until output, where it becomes U+FFFD (as in goldmark)
	p.lineBlank = append(p.lineBlank, isBlank(ln))

	for c := container.last; c != nil && c.open; c = container.last {
		container = c
		p.findNextNonspace()
		switch p.continues(container) {
		case 1:
			container = container.parent
		case 2:
			return
		default:
			p.lineBlank = append(p.lineBlank, isBlank(p.line[min(p.offset, len(p.line)):]))
			continue
		}
		break
	}
	p.allClosed = container == p.oldtip
	p.lastMatched = container

	matchedLeaf := container.kind != kParagraph && acceptsLines(container.kind)
	for !matchedLeaf {
		p.findNextNonspace()
		if !p.indented && !maybeSpecial(p.peek(p.nextNonspace)) {
			p.advanceNextNonspace()
			break
		}
		res := p.blockStart(container)
		if res == 0 {
			p.advanceNextNonspace()
			break
		}
		container = p.tip
		if res == 2 {
			matchedLeaf = true
		}
	}

	if !p.allClosed && !p.blank && p.tip.kind == kParagraph {
		p.addLine() // lazy paragraph continuation
		return
	}
	p.closeUnmatched()
	switch {
	case acceptsLines(container.kind):
		p.addLine()
		if container.kind == kHTMLBlock && container.htmlType >= 1 && container.htmlType <= 5 && htmlBlockEnds(container.htmlType, p.line[min(p.offset, len(p.line)):]) {
			p.finalize(container, p.lineNumber)
		}
	case p.offset < len(p.line) && !p.blank:
		p.addChild(kParagraph)
		p.advanceNextNonspace()
		p.addLine()
	}
}

func maybeSpecial(c byte) bool {
	return strings.IndexByte("#`~*+_=<>-", c) >= 0 || c >= '0' && c <= '9'
}

func htmlBlockEnds(typ int, s string) bool {
	if typ == 1 {
		return reHTML1Close.MatchString(s)
	}
	return strings.Contains(s, htmlBlockClose[typ])
}

// blockStart tries to open a block at the line's next non-space character:
// 0 none, 1 a container (keep looking inside it), 2 a leaf.
func (p *parser) blockStart(container *node) int {
	rest := p.line[p.nextNonspace:]
	c := p.peek(p.nextNonspace)
	switch {
	case !p.indented && c == '>' && p.depth < maxDepth:
		p.advanceNextNonspace()
		p.advanceOffset(1, false)
		if isSpaceOrTab(p.peek(p.offset)) {
			p.advanceOffset(1, true)
		}
		p.closeUnmatched()
		p.addChild(kBlockQuote)
		return 1
	case !p.indented && c == '#':
		m := reATXHeading.FindString(rest)
		if m == "" {
			break
		}
		p.advanceNextNonspace()
		p.advanceOffset(len(m), false)
		p.closeUnmatched()
		h := p.addChild(kHeading)
		h.level = len(strings.TrimSpace(m))
		h.content = []byte(trimATXClosing(p.line[p.offset:]))
		p.advanceOffset(len(p.line)-p.offset, false)
		return 2
	case !p.indented && (c == '`' || c == '~'):
		m := reCodeFence.FindString(rest)
		if m == "" || c == '`' && strings.IndexByte(rest[len(m):], '`') >= 0 {
			break
		}
		p.closeUnmatched()
		n := p.addChild(kCodeBlock)
		n.fenced = true
		n.fenceLength = len(m)
		n.fenceChar = c
		n.fenceOffset = p.indent
		p.advanceNextNonspace()
		p.advanceOffset(len(m), false)
		return 2
	case !p.indented && c == '<':
		if t := p.htmlBlockType(rest); t > 0 {
			p.closeUnmatched()
			n := p.addChild(kHTMLBlock)
			n.htmlType = t
			return 2
		}
	}
	if !p.indented && container.kind == kParagraph && (c == '=' || c == '-') && reSetext.MatchString(rest) {
		if res := p.setext(container, c); res != 0 {
			return res
		}
	}
	if !p.indented && (c == '*' || c == '_' || c == '-') && reThematic.MatchString(rest) {
		p.closeUnmatched()
		p.addChild(kThematicBreak)
		p.advanceOffset(len(p.line)-p.offset, false)
		return 2
	}
	if (!p.indented || container.kind == kList) && p.depth < maxDepth-1 {
		if data := p.listMarker(container); data != nil {
			p.closeUnmatched()
			if p.tip.kind != kList || !listsMatch(container.list, data) {
				l := p.addChild(kList)
				l.list = data
			}
			item := p.addChild(kItem)
			item.list = data
			p.emptyItemBlank = false
			return 1
		}
	}
	if p.indented && p.tip.kind != kParagraph && !p.blank {
		p.advanceOffset(codeIndent, true)
		p.closeUnmatched()
		p.addChild(kCodeBlock)
		return 2
	}
	return 0
}

// trimATXClosing drops an ATX heading's closing #s.
func trimATXClosing(s string) string {
	t := strings.TrimRight(s, " \t")
	h := strings.TrimRight(t, "#")
	if len(h) == len(t) {
		return s
	}
	if h == "" || h[len(h)-1] == ' ' || h[len(h)-1] == '\t' {
		return strings.TrimRight(h, " \t")
	}
	return s
}

// htmlBlockType is goldmark's HTML block opener test on the line from its
// first non-space character.
func (p *parser) htmlBlockType(s string) int {
	switch {
	case reHTML1Open.MatchString(s):
		return 1
	case strings.HasPrefix(s, "<!--"):
		return 2
	case strings.HasPrefix(s, "<?"):
		return 3
	case reHTML4Open.MatchString(s):
		return 4
	case strings.HasPrefix(s, "<![CDATA["):
		return 5
	}
	if m := reHTML7.FindStringSubmatch(s); m != nil {
		closing := m[1] == "/"
		hasAttr := m[3] != ""
		name := strings.ToLower(m[2])
		if blockTags[name] {
			return 6
		}
		if name != "script" && name != "style" && name != "pre" && p.tip.kind != kParagraph && (!closing || !hasAttr) {
			return 7
		}
	}
	if m := reHTML6.FindStringSubmatch(s); m != nil && blockTags[strings.ToLower(m[1])] {
		return 6
	}
	return 0
}

// setext makes the paragraph a setext heading: 2 when it did, 1 when link
// reference definitions or a table took the whole paragraph and the line
// must be read again without it.
func (p *parser) setext(container *node, c byte) int {
	p.closeUnmatched()
	rest := p.paragraphRefs(container)
	after := container
	if p.gfm {
		// goldmark closes the paragraph first, which trims it, and then looks
		// for a table: one from the first line leaves no heading, one after
		// some lines puts the heading, made of those lines, after the table.
		trimmed := strings.TrimRight(rest, " \t\n\r")
		switch t := tableAt(trimmed, nil); {
		case t == 0:
			p.table(container, trimmed, nil)
			rest = ""
		case t > 0:
			rest = p.table(container, trimmed, nil)
			after = container.next
		}
	}
	if isBlank(rest) {
		// Nothing left of the paragraph: goldmark reads the line again
		// without it.
		container.open = false
		container.endLine = p.lineNumber - 1
		p.tip = container.parent
		p.dropParagraph(container)
		return 1
	}
	h := &node{kind: kHeading, startLine: container.startLine, open: true, blankBefore: container.blankBefore}
	h.level = 1
	if c == '-' {
		h.level = 2
	}
	h.content = []byte(rest)
	h.before = container.before
	after.insertAfter(h)
	container.unlink()
	p.tip = h
	p.advanceOffset(len(p.line)-p.offset, false)
	return 2
}

// listMarker reads a list marker at the next non-space character.
func (p *parser) listMarker(container *node) *listData {
	if p.indent >= codeIndent {
		return nil
	}
	rest := p.line[p.nextNonspace:]
	data := &listData{tight: true, offset: p.indent}
	var marker int
	if m := reBullet.FindString(rest); m != "" {
		data.bulletChar = m[0]
		marker = 1
	} else if m := reOrdered.FindStringSubmatch(rest); m != nil && (container.kind != kParagraph || m[1] == "1") {
		data.ordered = true
		for _, d := range m[1] {
			data.start = data.start*10 + int(d-'0')
		}
		data.delimiter = m[2][0]
		marker = len(m[0])
	} else {
		return nil
	}
	if i := p.nextNonspace + marker; i < len(p.line) && p.line[i] != ' ' && p.line[i] != '\t' || i == len(p.line) && p.lineCR {
		return nil // a marker needs a space or the line end after it (for goldmark, \r is neither)
	}
	if container.kind == kParagraph && isBlank(p.line[p.nextNonspace+marker:]) {
		return nil
	}
	p.advanceNextNonspace()
	p.advanceOffset(marker, true)
	startCol, startOff := p.column, p.offset
	for {
		p.advanceOffset(1, true)
		if p.column-startCol >= 5 || !isSpaceOrTab(p.peek(p.offset)) {
			break
		}
	}
	blankItem := p.offset >= len(p.line)
	spaces := p.column - startCol
	if spaces >= 5 || spaces < 1 || blankItem {
		data.padding = marker + 1
		p.column, p.offset = startCol, startOff
		if isSpaceOrTab(p.peek(p.offset)) {
			p.advanceOffset(1, true)
		}
	} else {
		data.padding = marker + spaces
	}
	return data
}

// listGoesOn is goldmark's test for a list on a line after an empty item
// and a blank line: only one of its own items keeps it open.
func (p *parser) listGoesOn(l *node) bool {
	last := l.last
	if last == nil || p.indent >= codeIndent {
		return false
	}
	offset := last.list.offset + last.list.padding
	if p.indent >= offset && last.first != nil {
		return false
	}
	rest := p.line[p.nextNonspace:]
	if reThematic.MatchString(rest) {
		return false
	}
	data := markerAt(rest)
	return data != nil && listsMatch(l.list, data)
}

// markerAt reads the list marker s starts with: its kind only.
func markerAt(s string) *listData {
	var data listData
	n := 0
	if m := reBullet.FindString(s); m != "" {
		data.bulletChar = m[0]
		n = 1
	} else if m := reOrdered.FindStringSubmatch(s); m != nil {
		data.ordered = true
		data.delimiter = m[2][0]
		n = len(m[0])
	} else {
		return nil
	}
	if n < len(s) && s[n] != ' ' && s[n] != '\t' {
		return nil
	}
	return &data
}

func listsMatch(a, b *listData) bool {
	return a != nil && a.ordered == b.ordered && a.delimiter == b.delimiter && a.bulletChar == b.bulletChar
}

func (p *parser) finalize(n *node, line int) {
	above := n.parent
	n.open = false
	n.endLine = line
	if isContainer(n.kind) {
		p.depth--
	}
	switch n.kind {
	case kParagraph:
		p.finalizeParagraph(n)
	case kCodeBlock:
		s := string(n.content)
		if n.fenced {
			first, rest, _ := strings.Cut(s, "\n")
			n.info = unescape(trimSpace(first))
			n.literal = rest
		} else {
			n.literal = trimBlankLines(s) + "\n"
			// The block ends at its last line of code: blank lines after it
			// separate it from what follows (a list with them is loose).
			n.endLine -= strings.Count(s, "\n") - strings.Count(n.literal, "\n")
		}
		n.content = nil
	case kHTMLBlock:
		// goldmark writes the lines as they are, up to how the input ends.
		n.literal = string(n.content)
		if p.noFinalNewline && n.lastText == p.lineCount {
			n.literal = strings.TrimSuffix(n.literal, "\n")
		}
		n.content = nil
	case kItem:
		if n.last != nil {
			n.endLine = n.last.endLine
		} else {
			n.endLine = n.startLine
		}
	case kList:
		p.finalizeList(n)
	}
	p.tip = above
}

// finalizeList decides tightness as goldmark does: a list is loose when
// an item after the first, or a block after an item's first, follows a
// blank line.
func (p *parser) finalizeList(n *node) {
	for item := n.first; item != nil && n.list.tight; item = item.next {
		if item != n.first && item.blankBefore {
			n.list.tight = false
		}
		if item.first != nil {
			for c := item.first.next; c != nil; c = c.next {
				if c.blankBefore {
					n.list.tight = false
				}
			}
		}
	}
	if n.last != nil {
		n.endLine = n.last.endLine
	}
}

func (p *parser) finalizeParagraph(n *node) {
	rest := p.paragraphRefs(n)
	if p.gfm {
		if p.noFinalNewline && n.lastText == p.lineCount {
			rest = strings.TrimSuffix(rest, "\n")
		}
		rest = p.table(n, rest, n.indents)
	}
	if isBlank(rest) {
		p.dropParagraph(n)
		return
	}
	n.content = []byte(rest)
}

// dropParagraph removes a paragraph nothing is left of. When link
// reference definitions made it up, a placeholder stays: goldmark keeps
// definitions as blocks, and they count for a list's tightness.
func (p *parser) dropParagraph(n *node) {
	if n.refs {
		n.kind = kRefDefs
		n.content = nil
		return
	}
	n.unlink()
}

// paragraphRefs takes the link reference definitions off the start of a
// paragraph and returns what is left (the paragraph's remaining lines).
func (p *parser) paragraphRefs(n *node) string {
	s := string(n.content)
	for {
		l, ok := p.linkRefDef(s)
		if !ok {
			break
		}
		lines := strings.Count(s[:l], "\n")
		n.before = n.before[lines:]
		n.indents = n.indents[lines:]
		s = s[l:]
		n.refs = true
	}
	n.content = []byte(s)
	return s
}

func lastRune(s string) rune {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] < 0x80 || s[i] >= 0xC0 {
			r := []rune(s[i:])
			return r[0]
		}
	}
	return '\n'
}

// trimBlankLines drops the line ends and space-only lines at the end of s.
func trimBlankLines(s string) string {
	end := len(s)
	for end > 0 {
		i := strings.LastIndexByte(s[:end], '\n')
		if i < 0 || strings.Trim(s[i+1:end], " ") != "" {
			break
		}
		end = i
	}
	return s[:end]
}
