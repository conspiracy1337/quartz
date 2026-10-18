// Package markdown converts Markdown (CommonMark plus GitHub's tables,
// strikethrough, task lists and bare links) to HTML. For descriptions it
// reads the way goldmark does, which docs relied on before; this package
// replaced it because goldmark cost about 850 KB of binary size.
//
// Raw HTML in the input passes through unchanged and link destinations are
// not filtered: the result must be sanitized before it is shown, as
// docs.HTML does.
package markdown

import (
	"strconv"
	"strings"
)

// HTML converts Markdown to HTML.
func HTML(src string) string {
	return render((&parser{gfm: true}).parse(src))
}

// commonMark converts without the GitHub extensions (for the spec tests).
func commonMark(src string) string {
	return render((&parser{}).parse(src))
}

type renderer struct {
	out strings.Builder
}

// raw writes an HTML block as it is, but for NUL (as goldmark).
func (r *renderer) raw(s string) {
	r.out.WriteString(strings.ReplaceAll(s, "\x00", "\uFFFD"))
}

// cr starts a new line unless the output is at one.
func (r *renderer) cr() {
	if s := r.out.String(); s != "" && s[len(s)-1] != '\n' {
		r.out.WriteByte('\n')
	}
}

func render(doc *node) string {
	var r renderer
	// Walk the tree without recursion: deep nesting can't exhaust the stack.
	n, entering := doc, true
	for n != nil {
		if entering {
			if r.enter(n) && n.first != nil {
				n = n.first
				continue
			}
			if n.first != nil || isContainerKind(n.kind) {
				r.exit(n)
			}
		} else {
			r.exit(n)
		}
		if n == doc {
			break
		}
		if n.next != nil {
			n, entering = n.next, true
		} else {
			n, entering = n.parent, false
		}
	}
	return r.out.String()
}

func isContainerKind(k kind) bool {
	switch k {
	case kText, kSoftBreak, kHardBreak, kCode, kHTMLInline, kCheckbox, kThematicBreak, kCodeBlock, kHTMLBlock, kImage, kRefDefs:
		return false
	}
	return true
}

func tightParagraph(n *node) bool {
	if n.parent == nil || n.parent.parent == nil {
		return false
	}
	gp := n.parent.parent
	return gp.kind == kList && gp.list.tight
}

// enter writes the start of n and reports whether to visit its children.
func (r *renderer) enter(n *node) bool {
	switch n.kind {
	case kDocument:
	case kParagraph:
		if !tightParagraph(n) {
			r.cr()
			r.out.WriteString("<p>")
		}
	case kHeading:
		r.cr()
		r.out.WriteString("<h" + strconv.Itoa(n.level) + ">")
	case kThematicBreak:
		r.cr()
		r.out.WriteString("<hr />")
		r.cr()
	case kBlockQuote:
		r.cr()
		r.out.WriteString("<blockquote>")
		r.cr()
	case kList:
		r.cr()
		if n.list.ordered {
			if n.list.start != 1 {
				r.out.WriteString(`<ol start="` + strconv.Itoa(n.list.start) + `">`)
			} else {
				r.out.WriteString("<ol>")
			}
		} else {
			r.out.WriteString("<ul>")
		}
		r.cr()
	case kItem:
		r.out.WriteString("<li>")
	case kCodeBlock:
		r.cr()
		r.out.WriteString("<pre><code")
		if lang, _, _ := strings.Cut(n.info, " "); lang != "" {
			r.out.WriteString(` class="language-`)
			escapeHTML(&r.out, lang)
			r.out.WriteByte('"')
		}
		r.out.WriteByte('>')
		escapeHTML(&r.out, n.literal)
		r.out.WriteString("</code></pre>")
		r.cr()
	case kHTMLBlock:
		r.cr()
		r.raw(n.literal)
	case kTable:
		r.cr()
		r.out.WriteString("<table>\n")
	case kTableRow:
		if n.header {
			r.out.WriteString("<thead>\n")
		} else if n.prev != nil && n.prev.header {
			r.out.WriteString("<tbody>\n")
		}
		r.out.WriteString("<tr>\n")
	case kTableCell:
		r.out.WriteString("<" + cellTag(n))
		switch n.align {
		case 'l':
			r.out.WriteString(` align="left"`)
		case 'c':
			r.out.WriteString(` align="center"`)
		case 'r':
			r.out.WriteString(` align="right"`)
		}
		r.out.WriteByte('>')
	case kText:
		escapeHTML(&r.out, n.literal)
	case kSoftBreak:
		r.out.WriteByte('\n')
	case kHardBreak:
		r.out.WriteString("<br />\n")
	case kCode:
		r.out.WriteString("<code>")
		escapeHTML(&r.out, n.literal)
		r.out.WriteString("</code>")
	case kHTMLInline:
		r.out.WriteString(n.literal)
	case kCheckbox:
		if n.checked {
			r.out.WriteString(`<input checked="" disabled="" type="checkbox" /> `)
		} else {
			r.out.WriteString(`<input disabled="" type="checkbox" /> `)
		}
	case kEmph:
		r.out.WriteString("<em>")
	case kStrong:
		r.out.WriteString("<strong>")
	case kDel:
		r.out.WriteString("<del>")
	case kLink:
		r.out.WriteString(`<a href="`)
		escapeHTML(&r.out, linkDest(n))
		r.out.WriteByte('"')
		if n.title != "" {
			r.out.WriteString(` title="`)
			escapeHTML(&r.out, unescape(n.title))
			r.out.WriteByte('"')
		}
		r.out.WriteByte('>')
	case kImage:
		r.out.WriteString(`<img src="`)
		escapeHTML(&r.out, linkDest(n))
		r.out.WriteString(`" alt="`)
		escapeHTML(&r.out, plainText(n))
		r.out.WriteByte('"')
		if n.title != "" {
			r.out.WriteString(` title="`)
			escapeHTML(&r.out, unescape(n.title))
			r.out.WriteByte('"')
		}
		r.out.WriteString(" />")
		return false
	}
	return true
}

func (r *renderer) exit(n *node) {
	switch n.kind {
	case kParagraph:
		if !tightParagraph(n) {
			r.out.WriteString("</p>")
			r.cr()
		}
	case kHeading:
		r.out.WriteString("</h" + strconv.Itoa(n.level) + ">")
		r.cr()
	case kBlockQuote:
		r.cr()
		r.out.WriteString("</blockquote>")
		r.cr()
	case kList:
		r.cr()
		if n.list.ordered {
			r.out.WriteString("</ol>")
		} else {
			r.out.WriteString("</ul>")
		}
		r.cr()
	case kItem:
		r.out.WriteString("</li>")
		r.cr()
	case kTable:
		if n.last != nil && !n.last.header {
			r.out.WriteString("</tbody>\n")
		}
		r.out.WriteString("</table>\n")
	case kTableRow:
		r.out.WriteString("</tr>\n")
		if n.header {
			r.out.WriteString("</thead>\n")
		}
	case kTableCell:
		r.out.WriteString("</" + cellTag(n) + ">\n")
	case kEmph:
		r.out.WriteString("</em>")
	case kStrong:
		r.out.WriteString("</strong>")
	case kDel:
		r.out.WriteString("</del>")
	case kLink:
		r.out.WriteString("</a>")
	}
}

func cellTag(n *node) string {
	if n.parent != nil && n.parent.header {
		return "th"
	}
	return "td"
}

// linkDest is the address a link or picture points at, escaped as goldmark
// does: written destinations have their escapes and references resolved,
// autolinks are taken as they are.
func linkDest(n *node) string {
	if n.autolink {
		return urlEscape(n.dest, false)
	}
	return urlEscape(n.dest, true)
}

// plainText is the text inside n, for a picture's alt text.
func plainText(n *node) string {
	var b strings.Builder
	var walk func(*node)
	walk = func(n *node) {
		for c := n.first; c != nil; c = c.next {
			switch c.kind {
			case kText, kCode:
				b.WriteString(c.literal)
			case kSoftBreak:
				b.WriteByte('\n')
			case kHardBreak:
				b.WriteString("<br>\n") // goldmark writes the break into the alt text
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return b.String()
}
