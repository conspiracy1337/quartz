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
	strings.Builder
}

// raw writes an HTML block as it is, but for NUL (as goldmark).
func (r *renderer) raw(s string) {
	r.WriteString(strings.ReplaceAll(s, "\x00", "\uFFFD"))
}

// cr starts a new line unless the output is at one.
func (r *renderer) cr() {
	if s := r.String(); s != "" && s[len(s)-1] != '\n' {
		r.WriteByte('\n')
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
	return r.String()
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
			r.WriteString("<p>")
		}
	case kHeading:
		r.cr()
		r.WriteString("<h" + strconv.Itoa(n.level) + ">")
	case kThematicBreak:
		r.cr()
		r.WriteString("<hr />")
		r.cr()
	case kBlockQuote:
		r.cr()
		r.WriteString("<blockquote>")
		r.cr()
	case kList:
		r.cr()
		if n.list.ordered {
			if n.list.start != 1 {
				r.WriteString(`<ol start="` + strconv.Itoa(n.list.start) + `">`)
			} else {
				r.WriteString("<ol>")
			}
		} else {
			r.WriteString("<ul>")
		}
		r.cr()
	case kItem:
		r.WriteString("<li>")
	case kCodeBlock:
		r.cr()
		r.WriteString("<pre><code")
		if lang, _, _ := strings.Cut(n.info, " "); lang != "" {
			r.WriteString(` class="language-`)
			escapeHTML(&r.Builder, lang)
			r.WriteByte('"')
		}
		r.WriteByte('>')
		escapeHTML(&r.Builder, n.literal)
		r.WriteString("</code></pre>")
		r.cr()
	case kHTMLBlock:
		r.cr()
		r.raw(n.literal)
	case kTable:
		r.cr()
		r.WriteString("<table>\n")
	case kTableRow:
		if n.header {
			r.WriteString("<thead>\n")
		} else if n.prev != nil && n.prev.header {
			r.WriteString("<tbody>\n")
		}
		r.WriteString("<tr>\n")
	case kTableCell:
		r.WriteString("<" + cellTag(n))
		switch n.align {
		case 'l':
			r.WriteString(` align="left"`)
		case 'c':
			r.WriteString(` align="center"`)
		case 'r':
			r.WriteString(` align="right"`)
		}
		r.WriteByte('>')
	case kText:
		escapeHTML(&r.Builder, n.literal)
	case kSoftBreak:
		r.WriteByte('\n')
	case kHardBreak:
		r.WriteString("<br />\n")
	case kCode:
		r.WriteString("<code>")
		escapeHTML(&r.Builder, n.literal)
		r.WriteString("</code>")
	case kHTMLInline:
		r.WriteString(n.literal)
	case kCheckbox:
		if n.checked {
			r.WriteString(`<input checked="" disabled="" type="checkbox" /> `)
		} else {
			r.WriteString(`<input disabled="" type="checkbox" /> `)
		}
	case kEmph:
		r.WriteString("<em>")
	case kStrong:
		r.WriteString("<strong>")
	case kDel:
		r.WriteString("<del>")
	case kLink:
		r.WriteString(`<a href="`)
		escapeHTML(&r.Builder, linkDest(n))
		r.WriteByte('"')
		if n.title != "" {
			r.WriteString(` title="`)
			escapeHTML(&r.Builder, unescape(n.title))
			r.WriteByte('"')
		}
		r.WriteByte('>')
	case kImage:
		r.WriteString(`<img src="`)
		escapeHTML(&r.Builder, linkDest(n))
		r.WriteString(`" alt="`)
		escapeHTML(&r.Builder, plainText(n))
		r.WriteByte('"')
		if n.title != "" {
			r.WriteString(` title="`)
			escapeHTML(&r.Builder, unescape(n.title))
			r.WriteByte('"')
		}
		r.WriteString(" />")
		return false
	}
	return true
}

func (r *renderer) exit(n *node) {
	switch n.kind {
	case kParagraph:
		if !tightParagraph(n) {
			r.WriteString("</p>")
			r.cr()
		}
	case kHeading:
		r.WriteString("</h" + strconv.Itoa(n.level) + ">")
		r.cr()
	case kBlockQuote:
		r.cr()
		r.WriteString("</blockquote>")
		r.cr()
	case kList:
		r.cr()
		if n.list.ordered {
			r.WriteString("</ol>")
		} else {
			r.WriteString("</ul>")
		}
		r.cr()
	case kItem:
		r.WriteString("</li>")
		r.cr()
	case kTable:
		if n.last != nil && !n.last.header {
			r.WriteString("</tbody>\n")
		}
		r.WriteString("</table>\n")
	case kTableRow:
		r.WriteString("</tr>\n")
		if n.header {
			r.WriteString("</thead>\n")
		}
	case kTableCell:
		r.WriteString("</" + cellTag(n) + ">\n")
	case kEmph:
		r.WriteString("</em>")
	case kStrong:
		r.WriteString("</strong>")
	case kDel:
		r.WriteString("</del>")
	case kLink:
		r.WriteString("</a>")
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
