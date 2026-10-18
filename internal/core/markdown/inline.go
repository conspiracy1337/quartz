package markdown

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// The inline parser walks a block's text the way goldmark does: a character
// can start an inline element when it is punctuation (and not escaped), a
// space, or the first character of a line or after another element. That
// last rule decides where GFM's bare links may start. Emphasis and links
// use the CommonMark delimiter and bracket algorithms.

var (
	attributePattern = `(?:[\r\n \t]+[a-zA-Z_:][a-zA-Z0-9:._-]*(?:[\r\n \t]*=[\r\n \t]*(?:[^\"'=<>` + "`" + `\x00-\x20]+|'[^']*'|"[^"]*"))?)`
	reOpenTag        = regexp.MustCompile(`^<[A-Za-z][A-Za-z0-9-]*` + attributePattern + `*(?:[ \t]|(?:\r\n|\n){0,1})*/?>`)
	reCloseTag       = regexp.MustCompile(`^</[A-Za-z][A-Za-z0-9-]*(?:[ \t]|(?:\r\n|\n){0,1})*>`)
	reAutolinkURL    = regexp.MustCompile(`^<([A-Za-z][A-Za-z0-9.+-]{1,31}:[^<>\x00-\x20]*)>`)
	reAutolinkEmail  = regexp.MustCompile(`^<([a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@` + emailDomain + `)>`)
	reEmailDomain    = regexp.MustCompile(`^` + emailDomain)
	emailDomain      = `[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*`
	// goldmark's linkify patterns.
	reLinkifyURL = regexp.MustCompile(`^(?:http|https|ftp)://[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-z]+(?::\d+)?(?:[/#?][-a-zA-Z0-9@:%_+.~#$!?&/=\(\);,'">\^{}\[\]` + "`" + `]*)?`)
	reLinkifyWWW = regexp.MustCompile(`^www\.[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-z]+(?:[/#?][-a-zA-Z0-9@:%_\+.~#!?&/=\(\);,'">\^{}\[\]` + "`" + `]*)?`)
)

type delim struct {
	node               *node
	ch                 byte
	length, origLength int
	canOpen, canClose  bool
	prev, next         *delim
}

type bracket struct {
	node       *node
	prev       *bracket
	prevDelim  *delim
	start, end int // the "[" (or "![") in the text
	image      bool
	active     bool
}

type inline struct {
	p          *parser
	s          string
	pos        int
	block      *node
	lineStarts []int
	before     []rune
	buf        strings.Builder
	delims     *delim
	brackets   *bracket
	oldest     *bracket // bottom of the bracket stack
	head       bool     // at a line's start or right after an element
	cell       bool     // a table cell: "\|" in code is a pipe
	task       bool     // a task list checkbox may start here
	// escaped: goldmark's escape state outlived a line that ended with a
	// backslash and a two-space hard break, so a backslash starting the
	// next line is a plain character.
	escaped bool
	// Searches that found nothing are remembered, so text that keeps
	// opening comments or code spans stays linear.
	noClose [6]bool
	noTicks map[int]bool
}

func (p *parser) inlines(n *node) {
	for c := n.first; c != nil; c = c.next {
		p.inlines(c)
	}
	switch n.kind {
	case kParagraph, kHeading, kTableCell:
	default:
		return
	}
	in := &inline{p: p, block: n, s: strings.Trim(string(n.content), " \t\n\r"), before: n.before, cell: n.kind == kTableCell}
	n.content = nil
	in.task = p.gfm && n.kind == kParagraph && n.parent != nil && n.parent.kind == kItem && n.parent.first == n
	in.lineStarts = append(in.lineStarts, 0)
	for i := 0; i < len(in.s); i++ {
		if in.s[i] == '\n' {
			in.lineStarts = append(in.lineStarts, i+1)
		}
	}
	in.parse()
}

func (in *inline) parse() {
	in.head = true
	for in.pos < len(in.s) {
		in.step()
	}
	in.flush()
	in.processEmphasis(nil)
}

// flush ends the text collected so far.
func (in *inline) flush() {
	if in.buf.Len() > 0 {
		in.block.appendChild(text(in.buf.String()))
		in.buf.Reset()
	}
}

func (in *inline) add(n *node) {
	in.flush()
	in.block.appendChild(n)
}

// before returns the character before position i, as goldmark sees it: a
// line's start is preceded by what came before it in the source.
func (in *inline) charBefore(i int) rune {
	if i == 0 {
		return '\n'
	}
	if in.s[i-1] == '\n' {
		line := sort.SearchInts(in.lineStarts, i+1) - 1
		if line < len(in.before) {
			return in.before[line]
		}
		return '\n'
	}
	j := i - 1
	for j >= 0 && !utf8.RuneStart(in.s[j]) {
		j--
	}
	if j < 0 {
		return '\n'
	}
	r, _ := utf8.DecodeRuneInString(in.s[j:])
	return r
}

func (in *inline) step() {
	c := in.s[in.pos]
	head := in.head
	in.head = false
	if in.escaped && c != '\\' {
		in.escaped = false
	}
	if c == '\n' {
		in.newline()
		in.head = true
		return
	}
	gfm := in.p.gfm
	if in.task {
		in.task = false
		if in.checkbox() {
			in.head = true
			return
		}
	}
	if gfm && in.brackets == nil && (head && !isPunct(c) || c == ' ' || c == '\t') {
		skip := 0
		if c == ' ' {
			skip = 1
		}
		if in.linkify(in.pos+skip, skip) {
			return
		}
	}
	switch c {
	case '\\':
		in.backslash()
	case '`':
		in.codeSpan()
		in.head = true
	case '*', '_':
		in.delimRun(c)
		in.head = true
	case '~':
		if gfm && in.strike() {
			in.head = true
			return
		}
		if gfm && in.brackets == nil && in.linkify(in.pos+1, 1) {
			return
		}
		in.buf.WriteByte('~')
		in.pos++
	case '(':
		if gfm && in.brackets == nil && in.linkify(in.pos+1, 1) {
			return
		}
		in.buf.WriteByte('(')
		in.pos++
	case '[':
		in.openBracket(in.pos, false)
	case '!':
		if in.pos+1 < len(in.s) && in.s[in.pos+1] == '[' {
			in.openBracket(in.pos, true)
			return
		}
		in.buf.WriteByte('!')
		in.pos++
	case ']':
		in.closeBracket()
	case '<':
		if in.autolink() || in.rawHTML() {
			in.head = true
			return
		}
		in.buf.WriteByte('<')
		in.pos++
	case '&':
		if v, n, ok := entityAt(in.s, in.pos); ok {
			in.buf.WriteString(v)
			in.pos += n
			return
		}
		in.buf.WriteByte('&')
		in.pos++
	default:
		j := in.pos + 1
		for j < len(in.s) && !special(in.s[j]) {
			j++
		}
		in.buf.WriteString(in.s[in.pos:j])
		in.pos = j
	}
}

func special(c byte) bool {
	switch c {
	case '\n', '\\', '`', '*', '_', '~', '(', '[', '!', ']', '<', '&', ' ':
		return true
	}
	return false
}

// newline ends a line: a hard break after two spaces, else a soft one.
func (in *inline) newline() {
	t := in.buf.String()
	trimmed := strings.TrimRight(t, " \t")
	hard := in.pos >= 2 && in.s[in.pos-1] == ' ' && in.s[in.pos-2] == ' '
	in.buf.Reset()
	in.buf.WriteString(trimmed)
	if in.p.gfm && hard {
		// goldmark ends text at every space it tries a bare link after, and
		// trims only the last piece: of n spaces before the break, n-3 stay.
		k := len(t) - len(trimmed)
		if k > 3 {
			in.buf.WriteString(t[len(trimmed) : len(t)-3])
		}
	}
	in.escaped = hard && in.pos >= 3 && in.s[in.pos-3] == '\\' && backslashes(in.s[:in.pos-2])%2 == 1 &&
		(in.pos < 4 || in.s[in.pos-4] != ' ')
	in.pos++
	if hard {
		in.add(&node{kind: kHardBreak})
	} else {
		in.add(&node{kind: kSoftBreak})
	}
	for in.pos < len(in.s) && isSpaceOrTab(in.s[in.pos]) {
		in.pos++
	}
}

// backslashes counts the backslashes s ends with.
func backslashes(s string) int {
	n := 0
	for n < len(s) && s[len(s)-1-n] == '\\' {
		n++
	}
	return n
}

func (in *inline) backslash() {
	if in.escaped {
		in.escaped = false
		in.buf.WriteByte('\\')
		in.pos++
		return
	}
	in.pos++
	if in.pos < len(in.s) {
		c := in.s[in.pos]
		if c == '\n' && (in.pos < 2 || in.s[in.pos-2] != '\\') {
			in.pos++
			in.add(&node{kind: kHardBreak})
			for in.pos < len(in.s) && isSpaceOrTab(in.s[in.pos]) {
				in.pos++
			}
			in.head = true
			return
		}
		if isPunct(c) {
			in.buf.WriteByte(c)
			in.pos++
			return
		}
	}
	in.buf.WriteByte('\\')
}

func (in *inline) codeSpan() {
	start := in.pos
	n := 0
	for in.pos < len(in.s) && in.s[in.pos] == '`' {
		in.pos++
		n++
	}
	if !in.noTicks[n] {
		for i := in.pos; i < len(in.s); {
			if in.s[i] != '`' {
				i++
				continue
			}
			j := i
			for j < len(in.s) && in.s[j] == '`' {
				j++
			}
			if j-i == n {
				in.add(&node{kind: kCode, literal: codeText(in.s[in.pos:i], in.cell)})
				in.pos = j
				return
			}
			i = j
		}
		if in.noTicks == nil {
			in.noTicks = map[int]bool{}
		}
		in.noTicks[n] = true
	}
	in.add(text(in.s[start:in.pos]))
}

// codeText is a code span's content: line ends become spaces, and one
// space comes off each end when both ends have one.
func codeText(s string, cell bool) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if cell {
		s = strings.ReplaceAll(s, `\|`, "|")
	}
	if len(s) >= 2 && s[0] == ' ' && s[len(s)-1] == ' ' && strings.Trim(s, " ") != "" {
		s = s[1 : len(s)-1]
	}
	return s
}

// scanDelims is goldmark's ScanDelimiter: the run's length and whether it
// may open or close emphasis.
func (in *inline) scanDelims(c byte) (n int, canOpen, canClose bool) {
	for in.pos+n < len(in.s) && in.s[in.pos+n] == c {
		n++
	}
	before := in.charBefore(in.pos)
	after := ' '
	if j := in.pos + n; j < len(in.s) {
		// goldmark's util.ToRune: from a continuation byte, back to where its rune starts.
		for j > 0 && !utf8.RuneStart(in.s[j]) {
			j--
		}
		after, _ = utf8.DecodeRuneInString(in.s[j:])
	}
	beforePunct, beforeSpace := isPunctRune(before), isSpaceRune(before)
	afterPunct, afterSpace := isPunctRune(after), isSpaceRune(after)
	left := !afterSpace && (!afterPunct || beforeSpace || beforePunct)
	right := !beforeSpace && (!beforePunct || afterSpace || afterPunct)
	if c == '_' {
		return n, left && (!right || beforePunct), right && (!left || afterPunct)
	}
	return n, left, right
}

func (in *inline) delimRun(c byte) {
	n, canOpen, canClose := in.scanDelims(c)
	in.pushDelim(c, n, canOpen, canClose)
}

func (in *inline) pushDelim(c byte, n int, canOpen, canClose bool) {
	t := text(in.s[in.pos : in.pos+n])
	in.add(t)
	in.pos += n
	d := &delim{node: t, ch: c, length: n, origLength: n, canOpen: canOpen, canClose: canClose, prev: in.delims}
	if in.delims != nil {
		in.delims.next = d
	}
	in.delims = d
}

// strike reads a "~" or "~~" run (GFM strikethrough).
func (in *inline) strike() bool {
	if in.charBefore(in.pos) == '~' {
		return false
	}
	n, canOpen, canClose := in.scanDelims('~')
	if n > 2 {
		return false
	}
	in.pushDelim('~', n, canOpen, canClose)
	return true
}

func (in *inline) removeDelim(d *delim) {
	if d.prev != nil {
		d.prev.next = d.next
	}
	if d.next != nil {
		d.next.prev = d.prev
	} else {
		in.delims = d.prev
	}
}

// processEmphasis matches the delimiters above bottom into emphasis,
// strong emphasis and strikethrough (CommonMark's algorithm, with the
// openers-bottom shortcut that keeps it linear).
func (in *inline) processEmphasis(bottom *delim) {
	var openersBottom [3][6]*delim
	for i := range openersBottom {
		for j := range openersBottom[i] {
			openersBottom[i][j] = bottom
		}
	}
	var closer *delim
	for d := in.delims; d != nil && d != bottom; d = d.prev {
		closer = d
	}
	for closer != nil {
		if !closer.canClose {
			closer = closer.next
			continue
		}
		ci := strings.IndexByte("*_~", closer.ch)
		bi := closer.origLength % 3
		if closer.canOpen {
			bi += 3
		}
		var opener *delim
		found := false
		for opener = closer.prev; opener != nil && opener != bottom && opener != openersBottom[ci][bi]; opener = opener.prev {
			if opener.ch == closer.ch && opener.canOpen {
				odd := (closer.canOpen || opener.canClose) && closer.origLength%3 != 0 && (opener.origLength+closer.origLength)%3 == 0
				if !odd {
					found = true
					break
				}
			}
		}
		old := closer
		if !found {
			openersBottom[ci][bi] = old.prev
			if !old.canOpen {
				in.removeDelim(old)
			}
			closer = closer.next
			continue
		}
		use := 1
		if closer.length >= 2 && opener.length >= 2 {
			use = 2
		}
		opener.length -= use
		closer.length -= use
		opener.node.literal = opener.node.literal[:opener.length]
		closer.node.literal = closer.node.literal[:closer.length]
		k := kEmph
		switch {
		case closer.ch == '~':
			k = kDel
		case use == 2:
			k = kStrong
		}
		em := &node{kind: k}
		for c := opener.node.next; c != nil && c != closer.node; {
			next := c.next
			c.unlink()
			em.appendChild(c)
			c = next
		}
		opener.node.insertAfter(em)
		for d := opener.next; d != nil && d != closer; {
			next := d.next
			in.removeDelim(d)
			d = next
		}
		if opener.length == 0 {
			opener.node.unlink()
			in.removeDelim(opener)
		}
		if closer.length == 0 {
			closer.node.unlink()
			next := closer.next
			in.removeDelim(closer)
			closer = next
		}
	}
	for in.delims != nil && in.delims != bottom {
		in.removeDelim(in.delims)
	}
}

func (in *inline) openBracket(at int, image bool) {
	in.pos = at + 1
	lit := "["
	if image {
		in.pos++
		lit = "!["
	}
	t := text(lit)
	in.add(t)
	b := &bracket{node: t, prev: in.brackets, prevDelim: in.delims, start: at, end: in.pos, image: image, active: true}
	if in.brackets == nil {
		in.oldest = b
	}
	in.brackets = b
	in.head = true
}

func (in *inline) closeBracket() {
	closeAt := in.pos
	in.pos++
	opener := in.brackets
	if opener == nil {
		in.buf.WriteByte(']')
		return
	}
	in.brackets = opener.prev
	fail := func() {
		in.buf.WriteByte(']')
	}
	if in.brackets != nil && in.brackets.end-in.oldest.start > 998 {
		fail()
		return
	}
	if !opener.active {
		fail()
		return
	}
	var dest, title string
	matched := false
	label := in.s[opener.end:closeAt]
	if in.pos < len(in.s) {
		switch in.s[in.pos] {
		case '(':
			if d, t, next, ok := in.inlineLink(in.pos); ok {
				dest, title, matched = d, t, true
				in.pos = next
			}
		case '[':
			if content, next, ok := findClosure(in.s, in.pos+1, '[', ']'); ok {
				if isBlank(content) {
					content = label
				}
				ref, found := in.lookup(content)
				if !found {
					fail()
					return
				}
				dest, title, matched = ref.dest, ref.title, true
				in.pos = next
			}
		}
	}
	if !matched {
		ref, found := in.lookup(label)
		if !found {
			fail()
			return
		}
		dest, title = ref.dest, ref.title
	}
	in.flush()
	k := kLink
	if opener.image {
		k = kImage
	}
	link := &node{kind: k, dest: dest, title: title}
	in.processEmphasis(opener.prevDelim)
	for c := opener.node.next; c != nil; {
		next := c.next
		c.unlink()
		link.appendChild(c)
		c = next
	}
	in.block.appendChild(link)
	opener.node.unlink()
	if !opener.image {
		for b := in.brackets; b != nil; b = b.prev {
			if !b.image {
				b.active = false
			}
		}
	}
	in.head = true
}

// lookup finds a link reference definition; labels over 999 bytes never match.
func (in *inline) lookup(label string) (linkRef, bool) {
	if len(label) > 999 {
		return linkRef{}, false
	}
	ref, ok := in.p.refs[normalizeLabel(label)]
	return ref, ok
}

// inlineLink reads "(destination title)" at s[i] == '(' as goldmark does.
func (in *inline) inlineLink(i int) (dest, title string, next int, ok bool) {
	s := in.s
	i = skipSpaces(s, i+1)
	if i < len(s) && s[i] == ')' {
		return "", "", i + 1, true
	}
	dest, i, ok = linkDestination(s, i)
	if !ok {
		return "", "", 0, false
	}
	i = skipSpaces(s, i)
	if i < len(s) && s[i] == ')' {
		return dest, "", i + 1, true
	}
	title, i, ok = linkTitle(s, i)
	if !ok {
		return "", "", 0, false
	}
	i = skipSpaces(s, i)
	if i < len(s) && s[i] == ')' {
		return dest, title, i + 1, true
	}
	return "", "", 0, false
}

func skipSpaces(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return i
}

func lineEnd(s string, i int) int {
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		return i + j + 1
	}
	return len(s)
}

// linkDestination is goldmark's parseLinkDestination on the rest of the line.
func linkDestination(s string, i int) (string, int, bool) {
	line := s[i:lineEnd(s, i)]
	if strings.HasPrefix(line, "<") {
		for j := 1; j < len(line); j++ {
			switch {
			case line[j] == '\\' && j < len(line)-1 && isPunct(line[j+1]):
				j++
			case line[j] == '>':
				return line[1:j], i + j + 1, true
			}
		}
		return "", 0, false
	}
	opened, j := 0, 0
loop:
	for j < len(line) {
		c := line[j]
		switch {
		case c == '\\' && j < len(line)-1 && isPunct(line[j+1]):
			j += 2
			continue
		case c == '(':
			opened++
		case c == ')':
			opened--
			if opened < 0 {
				break loop
			}
		case isSpace(c):
			break loop
		}
		j++
	}
	return line[:j], i + j, j != 0
}

func linkTitle(s string, i int) (string, int, bool) {
	if i >= len(s) {
		return "", 0, false
	}
	opener := s[i]
	closer := opener
	switch opener {
	case '"', '\'':
	case '(':
		closer = ')'
	default:
		return "", 0, false
	}
	return findClosure(s, i+1, opener, closer)
}

// findClosure finds the unescaped closer from s[i], across lines; an
// unescaped opener on the way fails (goldmark's FindClosure without nesting).
func findClosure(s string, i int, opener, closer byte) (string, int, bool) {
	for j := i; j < len(s); j++ {
		c := s[j]
		switch {
		case c == '\\' && j < len(s)-1 && isPunct(s[j+1]):
			j++
		case c == closer:
			return s[i:j], j + 1, true
		case c == opener:
			return "", 0, false
		}
	}
	return "", 0, false
}

// linkRefDef reads a link reference definition at the start of s (a
// paragraph's text) and returns the bytes it used, as goldmark does.
func (p *parser) linkRefDef(s string) (int, bool) {
	i := 0
	for i < len(s) && isSpaceOrTab(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != '[' {
		return 0, false
	}
	label, i, ok := findClosure(s, i+1, '[', ']')
	if !ok || isBlank(label) || i >= len(s) || s[i] != ':' {
		return 0, false
	}
	i = skipSpaces(s, i+1)
	dest, i, ok := linkDestination(s, i)
	if !ok {
		return 0, false
	}
	destLineEnd := lineEnd(s, i)
	newline := isBlank(s[i:destLineEnd])
	j := skipSpaces(s, i)
	title := ""
	end := destLineEnd
	if j < len(s) && (s[j] == '"' || s[j] == '\'' || s[j] == '(') && j > i {
		if t, k, found := linkTitle(s, j); found {
			if !isBlank(s[k:lineEnd(s, k)]) {
				if !newline {
					return 0, false
				}
				// goldmark keeps the title but leaves the destination's
				// line (unless it is the label's) in the paragraph.
				if start := strings.LastIndexByte(s[:i], '\n') + 1; start > 0 {
					end = start
				}
			} else {
				end = lineEnd(s, k)
			}
			title = t
		} else if !newline {
			return 0, false
		}
	} else if !newline {
		return 0, false
	}
	key := normalizeLabel(label)
	if _, dup := p.refs[key]; !dup {
		p.refs[key] = linkRef{dest: dest, title: title}
	}
	return end, true
}

// autolink reads <scheme:...> or <user@host>.
func (in *inline) autolink() bool {
	rest := in.s[in.pos:]
	if m := reAutolinkEmail.FindStringSubmatch(rest); m != nil {
		in.add(&node{kind: kLink, dest: "mailto:" + m[1], autolink: true, first: nil})
		in.block.last.appendChild(text(m[1]))
		in.pos += len(m[0])
		return true
	}
	if m := reAutolinkURL.FindStringSubmatch(rest); m != nil {
		in.add(&node{kind: kLink, dest: m[1], autolink: true})
		in.block.last.appendChild(text(m[1]))
		in.pos += len(m[0])
		return true
	}
	return false
}

// rawHTML reads an inline tag, comment, processing instruction,
// declaration or CDATA section with goldmark's rules.
func (in *inline) rawHTML() bool {
	rest := in.s[in.pos:]
	n := 0
	// goldmark matches tags through a reader that ends at U+FFFD.
	tag := rest
	if i := strings.IndexRune(tag, utf8.RuneError); i >= 0 {
		tag = tag[:i]
	}
	switch {
	case len(rest) > 1 && isAlnum(rest[1]):
		n = len(reOpenTag.FindString(tag))
	case len(rest) > 2 && rest[1] == '/' && isAlnum(rest[2]):
		n = len(reCloseTag.FindString(tag))
	case strings.HasPrefix(rest, "<!-->"):
		n = 5
	case strings.HasPrefix(rest, "<!--->"):
		n = 6
	case strings.HasPrefix(rest, "<!--"):
		n = in.until(rest, 4, "-->", 1)
	case strings.HasPrefix(rest, "<?"):
		n = in.until(rest, 0, "?>", 2) // from the "<", as goldmark: "<?>" is one
	case len(rest) > 2 && rest[1] == '!' && rest[2] >= 'A' && rest[2] <= 'Z':
		n = in.until(rest, 0, ">", 3)
	case strings.HasPrefix(rest, "<![CDATA["):
		n = in.until(rest, 0, "]]>", 4)
	}
	if n == 0 {
		return false
	}
	in.add(&node{kind: kHTMLInline, literal: rest[:n]})
	in.pos += n
	return true
}

// until is the length up to and including closer after from, or 0.
func (in *inline) until(rest string, from int, closer string, memo int) int {
	if in.noClose[memo] {
		return 0
	}
	if i := strings.Index(rest[from:], closer); i >= 0 {
		return from + i + len(closer)
	}
	in.noClose[memo] = true
	return 0
}

// checkbox reads a task list item's "[ ]" or "[x]".
func (in *inline) checkbox() bool {
	s := in.s[in.pos:]
	if len(s) < 3 || s[0] != '[' || s[2] != ']' {
		return false
	}
	switch s[1] {
	case 'x', 'X', ' ', '\t', '\n', '\r', '\f':
	default:
		return false
	}
	in.pos += 3
	for in.pos < len(in.s) && (isSpaceOrTab(in.s[in.pos]) || in.s[in.pos] == '\f') {
		in.pos++
	}
	in.add(&node{kind: kCheckbox, checked: s[1] == 'x' || s[1] == 'X'})
	return true
}

// linkify reads a bare URL, www. address or email address at s[at]
// (goldmark's Linkify); skip is how many characters before it (a space,
// "(" or "~") it takes along as text.
func (in *inline) linkify(at, skip int) bool {
	line := in.s[at:lineEnd(in.s, at)]
	var m []int
	protocol := ""
	if strings.HasPrefix(line, "http:") || strings.HasPrefix(line, "https:") || strings.HasPrefix(line, "ftp:") {
		m = reLinkifyURL.FindStringIndex(line)
	}
	if m == nil && strings.HasPrefix(line, "www.") {
		m = reLinkifyWWW.FindStringIndex(line)
		protocol = "http://"
	}
	email := false
	if m == nil {
		if len(line) > 0 && isPunct(line[0]) {
			return false
		}
		stop := findEmail(line)
		if stop < 0 {
			return false
		}
		atSign := strings.IndexByte(line, '@')
		if strings.IndexByte(line[atSign:stop-1], '.') < 0 {
			return false
		}
		if line[stop-1] == '.' {
			stop--
		}
		if stop < len(line) && (line[stop] == '-' || line[stop] == '_') {
			return false
		}
		m = []int{0, stop}
		email = true
	}
	i := m[1] - 1
	for ; i > 0; i-- {
		if strings.IndexByte("?!.,:*_~", line[i]) < 0 {
			break
		}
	}
	i++
	switch line[i-1] {
	case ')':
		i -= max(0, strings.Count(line[:i], ")")-strings.Count(line[:i], "("))
	case ';':
		j := i - 2
		for j >= 0 && isAlnum(line[j]) {
			j--
		}
		if j >= 0 && j != i-2 && line[j] == '&' {
			i = j
		}
	}
	if skip > 0 {
		in.buf.WriteString(in.s[at-skip : at])
	}
	url := line[:i]
	l := &node{kind: kLink, dest: protocol + url, autolink: true}
	if email {
		l.dest = "mailto:" + url
	}
	in.add(l)
	l.appendChild(text(url))
	in.pos = at + i
	in.head = true
	return true
}

// findEmail is goldmark's util.FindEmailIndex.
func findEmail(b string) int {
	i := 0
	for i < len(b) && emailLocal(b[i]) {
		i++
	}
	if i == 0 || i >= len(b) || b[i] != '@' {
		return -1
	}
	i++
	if i >= len(b) {
		return -1
	}
	m := reEmailDomain.FindStringIndex(b[i:])
	if m == nil {
		return -1
	}
	return i + m[1]
}

func emailLocal(c byte) bool {
	return isAlnum(c) || c != '"' && c != '(' && c != ')' && c != ',' && c != '<' && c != '>' && c != '@' &&
		c != '[' && c != '\\' && c != ']' && c != ':' && c != ';' && c > ' ' && c < 0x7f
}
