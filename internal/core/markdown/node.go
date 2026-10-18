package markdown

type kind uint8

const (
	kDocument kind = iota
	kParagraph
	kHeading
	kThematicBreak
	kBlockQuote
	kList
	kItem
	kCodeBlock
	kHTMLBlock
	kTable
	kTableRow
	kTableCell
	kRefDefs // link reference definitions: nothing to show, but goldmark keeps them as blocks

	kText
	kSoftBreak
	kHardBreak
	kCode
	kEmph
	kStrong
	kDel
	kLink
	kImage
	kHTMLInline
	kCheckbox
)

// node is one block or inline element. Blocks keep their raw text in
// content until inlines are parsed.
type node struct {
	kind                     kind
	parent, first, last      *node
	prev, next               *node
	open                     bool
	startLine, endLine       int
	lastText                 int  // the last line that added to content
	blankBefore              bool // the line before the block was blank (goldmark's tight lists)
	refs                     bool // paragraph: began with link reference definitions
	content                  []byte
	before                   []rune // per content line: the source character before it (emphasis flanking)
	indents                  []int  // per content line: its indentation in columns
	literal                  string // text, code, raw HTML, code block body
	level                    int    // heading level
	dest, title              string // link and image (dest raw, resolved at render)
	info                     string // fenced code info string
	list                     *listData
	fenced                   bool
	fenceChar                byte
	fenceLength, fenceOffset int
	htmlType                 int
	header                   bool // table row in the head
	align                    byte // table cell: 'l', 'c', 'r' or 0
	checked                  bool // task list checkbox
	autolink                 bool // link written as <…> or bare: dest is not resolved
}

type listData struct {
	ordered    bool
	tight      bool
	bulletChar byte
	start      int
	delimiter  byte
	padding    int
	offset     int // marker offset
}

func (n *node) appendChild(c *node) {
	c.parent = n
	c.prev = n.last
	c.next = nil
	if n.last != nil {
		n.last.next = c
	} else {
		n.first = c
	}
	n.last = c
}

func (n *node) insertAfter(s *node) {
	s.parent = n.parent
	s.prev = n
	s.next = n.next
	if n.next != nil {
		n.next.prev = s
	} else if n.parent != nil {
		n.parent.last = s
	}
	n.next = s
}

func (n *node) unlink() {
	if n.prev != nil {
		n.prev.next = n.next
	} else if n.parent != nil {
		n.parent.first = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else if n.parent != nil {
		n.parent.last = n.prev
	}
	n.parent, n.prev, n.next = nil, nil, nil
}

func text(s string) *node { return &node{kind: kText, literal: s} }
