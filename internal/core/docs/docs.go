// Package docs turns project descriptions (Modrinth's Markdown, CurseForge's
// HTML) into a few plain blocks the UI renders as text: headings, paragraphs,
// lists, pictures. No markup survives, so a description can't inject
// anything: text is text, links are https only, and pictures are kept only
// when the caller maps them to its own same-origin address.
package docs

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	maxBlocks = 400
	maxChars  = 40_000
	maxInput  = 2 << 20
)

// Span is a run of text: plain, code, or a link.
type Span struct {
	Text string `json:"text"`
	Code bool   `json:"code,omitempty"`
	Href string `json:"href,omitempty"`
	// Project names the project a link points at, when the caller knows the
	// address (set by callers, never by the parser).
	Project string `json:"project,omitempty"`
}

// Pic is one picture of a badge row.
type Pic struct {
	Src  string `json:"src"`
	Alt  string `json:"alt,omitempty"`
	Href string `json:"href,omitempty"`
}

// Block is a heading (h2), a paragraph (p), a list (ul, or numbered ol),
// preformatted text (pre, line breaks kept), a picture (img) or a row of
// small pictures such as badges (badges).
type Block struct {
	Type  string   `json:"type"`
	Spans []Span   `json:"spans,omitempty"`
	Items [][]Span `json:"items,omitempty"`
	Src   string   `json:"src,omitempty"`
	Alt   string   `json:"alt,omitempty"`
	Pics  []Pic    `json:"pics,omitempty"`

	href  string // the link around a picture
	small bool   // a picture sized 64px or less
	group int    // pictures in one group sit in one paragraph
}

// smallPic is the largest width or height a picture may declare to count as
// a badge.
const smallPic = 64

// Picture maps a picture's URL to the address the UI loads it from, or ""
// to drop it.
type Picture func(url string) string

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	// Raw HTML passes through to the HTML reader below, which keeps text only.
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// Markdown converts a Markdown description (raw HTML inside is allowed).
func Markdown(src string, pic Picture) []Block {
	if len(src) > maxInput {
		src = src[:maxInput]
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return nil
	}
	return HTML(buf.String(), pic)
}

// HTML converts an HTML description.
func HTML(src string, pic Picture) []Block {
	if len(src) > maxInput {
		src = src[:maxInput]
	}
	nodes, err := xhtml.ParseFragment(strings.NewReader(src), &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Body, Data: "body"})
	if err != nil {
		return nil
	}
	w := &walker{pic: pic}
	for _, n := range nodes {
		w.block(n)
	}
	w.flush()
	return badges(w.out)
}

// badges turns small pictures, and runs of two or more pictures in one
// paragraph, into badge rows (XMCL's grouping): a line of shields reads as
// one row instead of a column of large pictures.
func badges(in []Block) []Block {
	var out []Block
	for i := 0; i < len(in); {
		if in[i].Type != "img" {
			out = append(out, in[i])
			i++
			continue
		}
		j := i + 1
		for j < len(in) && in[j].Type == "img" && in[j].group == in[i].group {
			j++
		}
		run := in[i:j]
		allSmall := true
		for _, b := range run {
			allSmall = allSmall && b.small
		}
		if len(run) == 1 && !allSmall {
			out = append(out, run[0])
		} else {
			row := Block{Type: "badges"}
			for _, b := range run {
				row.Pics = append(row.Pics, Pic{Src: b.Src, Alt: b.Alt, Href: b.href})
			}
			out = append(out, row)
		}
		i = j
	}
	return out
}

type walker struct {
	pic   Picture
	out   []Block
	cur   []Span // the paragraph being collected
	chars int
	group int // bumped at every block boundary, so pictures know their paragraph
}

func (w *walker) full() bool { return len(w.out) >= maxBlocks || w.chars >= maxChars }

func (w *walker) add(b Block) {
	if w.full() {
		return
	}
	for _, s := range b.Spans {
		w.chars += utf8.RuneCountInString(s.Text)
	}
	for _, it := range b.Items {
		for _, s := range it {
			w.chars += utf8.RuneCountInString(s.Text)
		}
	}
	w.out = append(w.out, b)
}

// boundary ends a block: pictures on either side belong to different paragraphs.
func (w *walker) boundary() {
	w.flush()
	w.group++
}

// flush ends the paragraph being collected.
func (w *walker) flush() {
	if spans := tidy(w.cur); len(spans) > 0 {
		w.add(Block{Type: "p", Spans: spans})
	}
	w.cur = nil
}

// skipped elements never contribute text.
var skipped = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Iframe: true, atom.Noscript: true, atom.Svg: true, atom.Head: true,
	atom.Template: true, atom.Object: true, atom.Embed: true, atom.Video: true, atom.Audio: true, atom.Form: true,
	atom.Button: true, atom.Input: true, atom.Select: true, atom.Textarea: true, atom.Math: true,
}

// block reads n where blocks may start.
func (w *walker) block(n *xhtml.Node) {
	if w.full() {
		return
	}
	switch n.Type {
	case xhtml.TextNode:
		w.cur = append(w.cur, Span{Text: n.Data})
		return
	case xhtml.ElementNode:
	default:
		w.children(n)
		return
	}
	if skipped[n.DataAtom] {
		return
	}
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		w.boundary()
		if spans := tidy(w.inline(n)); len(spans) > 0 {
			w.add(Block{Type: "h2", Spans: spans})
		}
		w.boundary()
	case atom.Ul, atom.Ol:
		w.boundary()
		var items [][]Span
		w.items(n, &items)
		if len(items) > 0 {
			typ := "ul"
			if n.DataAtom == atom.Ol {
				typ = "ol"
			}
			w.add(Block{Type: typ, Items: items})
		}
		w.boundary()
	case atom.Pre:
		w.boundary()
		if t := strings.Trim(textOf(n), "\r\n"); strings.TrimSpace(t) != "" {
			w.add(Block{Type: "pre", Spans: []Span{{Text: t, Code: true}}})
		}
		w.boundary()
	case atom.Table:
		w.boundary()
		w.table(n)
		w.boundary()
	case atom.Img:
		w.image(n, "")
	case atom.Br:
		w.flush()
	case atom.Hr:
		w.boundary()
	case atom.P, atom.Div, atom.Blockquote, atom.Details, atom.Summary, atom.Section, atom.Article, atom.Center,
		atom.Header, atom.Footer, atom.Main, atom.Aside, atom.Figure, atom.Figcaption, atom.Dl, atom.Dt, atom.Dd, atom.Body:
		w.boundary()
		w.children(n)
		w.boundary()
	default:
		// Inline content (links, emphasis, code) joins the current paragraph;
		// a picture inside it still becomes its own block.
		w.cur = append(w.cur, w.inline(n)...)
	}
}

func (w *walker) children(n *xhtml.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.block(c)
	}
}

// inline collects n's text as spans; pictures inside become blocks.
func (w *walker) inline(n *xhtml.Node) []Span {
	var out []Span
	var walk func(*xhtml.Node, string, bool)
	walk = func(n *xhtml.Node, href string, code bool) {
		switch n.Type {
		case xhtml.TextNode:
			out = append(out, Span{Text: n.Data, Code: code, Href: href})
			return
		case xhtml.ElementNode:
		default:
			return
		}
		if skipped[n.DataAtom] {
			return
		}
		switch n.DataAtom {
		case atom.A:
			if u := safeLink(attr(n, "href")); u != "" {
				href = u
			}
		case atom.Code, atom.Kbd, atom.Samp:
			code = true
		case atom.Img:
			// Keep the paragraph's order: text before, the picture, text after.
			w.cur = append(w.cur, out...)
			out = nil
			w.image(n, href)
			return
		case atom.Br:
			out = append(out, Span{Text: " "})
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, href, code)
		}
	}
	walk(n, "", false)
	return out
}

// items reads list items; nested lists become further items.
func (w *walker) items(n *xhtml.Node, items *[][]Span) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != xhtml.ElementNode || c.DataAtom != atom.Li {
			continue
		}
		var spans []Span
		for g := c.FirstChild; g != nil; g = g.NextSibling {
			if g.Type == xhtml.ElementNode && (g.DataAtom == atom.Ul || g.DataAtom == atom.Ol) {
				if s := tidy(spans); len(s) > 0 {
					*items = append(*items, s)
				}
				spans = nil
				w.items(g, items)
				continue
			}
			if g.Type == xhtml.ElementNode && g.DataAtom == atom.P {
				spans = append(spans, Span{Text: " "})
			}
			spans = append(spans, w.inline(g)...)
		}
		if s := tidy(spans); len(s) > 0 {
			*items = append(*items, s)
		}
	}
}

// table writes each row as a paragraph, cells separated by a dot.
func (w *walker) table(n *xhtml.Node) {
	var rows func(*xhtml.Node)
	rows = func(n *xhtml.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != xhtml.ElementNode {
				continue
			}
			if c.DataAtom != atom.Tr {
				rows(c)
				continue
			}
			var spans []Span
			for cell := c.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.Type != xhtml.ElementNode || (cell.DataAtom != atom.Td && cell.DataAtom != atom.Th) {
					continue
				}
				s := tidy(w.inline(cell))
				if len(s) == 0 {
					continue
				}
				if len(spans) > 0 {
					spans = append(spans, Span{Text: " · "})
				}
				spans = append(spans, s...)
			}
			w.flush()
			if spans = tidy(spans); len(spans) > 0 {
				w.add(Block{Type: "p", Spans: spans})
			}
		}
	}
	rows(n)
}

func (w *walker) image(n *xhtml.Node, href string) {
	if w.pic == nil {
		return
	}
	src := w.pic(attr(n, "src"))
	if src == "" {
		return
	}
	w.flush()
	w.add(Block{Type: "img", Src: src, Alt: collapse(attr(n, "alt")), href: href, small: sized(attr(n, "width")) || sized(attr(n, "height")), group: w.group})
}

// sized reports whether an HTML width or height ("20", "20px") is small.
func sized(v string) bool {
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(v), "px"))
	return err == nil && n > 0 && n <= smallPic
}

func attr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func textOf(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == xhtml.ElementNode && skipped[n.DataAtom] {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// safeLink keeps https links only. CurseForge's /linkout?remoteUrl= wrapper
// is unwrapped to the address it leads to (Prism and XMCL do the same).
func safeLink(raw string) string {
	if u, err := url.Parse(raw); err == nil && strings.EqualFold(u.Hostname(), "www.curseforge.com") && u.Path == "/linkout" {
		inner := u.Query().Get("remoteUrl")
		if dec, err := url.QueryUnescape(inner); err == nil && strings.Contains(inner, "%") {
			inner = dec // CurseForge encodes the address twice
		}
		raw = inner
	}
	if len(raw) > 2048 || !strings.HasPrefix(strings.ToLower(raw), "https://") || strings.ContainsAny(raw, " \r\n\\") {
		return ""
	}
	return raw
}

// tidy collapses whitespace, merges neighbouring spans of the same kind and
// trims the ends; it returns nil when nothing visible is left.
func tidy(in []Span) []Span {
	var out []Span
	for _, s := range in {
		s.Text = collapse(s.Text)
		if s.Text == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Code == s.Code && out[n-1].Href == s.Href {
			out[n-1].Text += s.Text
			out[n-1].Text = strings.ReplaceAll(out[n-1].Text, "  ", " ")
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	out[0].Text = strings.TrimLeft(out[0].Text, " ")
	out[len(out)-1].Text = strings.TrimRight(out[len(out)-1].Text, " ")
	var kept []Span
	for _, s := range out {
		if s.Text != "" {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 || (len(kept) == 1 && strings.TrimSpace(kept[0].Text) == "") {
		return nil
	}
	return kept
}

// collapse turns runs of whitespace into one space, keeping a leading or
// trailing one so neighbouring spans don't run together.
func collapse(s string) string {
	if s == "" {
		return ""
	}
	f := strings.Fields(s)
	if len(f) == 0 {
		return " "
	}
	out := strings.Join(f, " ")
	if isSpace(s[0]) {
		out = " " + out
	}
	if isSpace(s[len(s)-1]) {
		out += " "
	}
	return out
}

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\t' || b == '\r' || b == '\f' }
