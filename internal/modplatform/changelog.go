package modplatform

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// A changelog becomes at most this many plain-text items for the dialog.
const (
	maxItems    = 12
	maxItemText = 240
)

var (
	mdLink   = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	mdMarks  = regexp.MustCompile("[*_`~]+")
	mdBullet = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	mdHead   = regexp.MustCompile(`^\s*#{1,6}\s*`)
	space    = regexp.MustCompile(`\s+`)
)

// Items turns a changelog into short plain-text lines: its bullet points,
// or its paragraphs when it has none. Nothing of the markup survives, so the
// UI never renders anything from a changelog but text.
func Items(raw string, isHTML bool) []string {
	if isHTML {
		return htmlItems(raw)
	}
	var bullets, paras []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if mdBullet.MatchString(line) {
			bullets = append(bullets, mdText(mdBullet.ReplaceAllString(line, "")))
		} else if t := mdText(mdHead.ReplaceAllString(line, "")); t != "" && !mdHead.MatchString(line) {
			paras = append(paras, t)
		}
	}
	if len(bullets) > 0 {
		return capItems(bullets)
	}
	return capItems(paras)
}

func mdText(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdMarks.ReplaceAllString(s, "")
	return strings.TrimSpace(space.ReplaceAllString(s, " "))
}

func htmlItems(raw string) []string {
	z := html.NewTokenizer(strings.NewReader(raw))
	var items, blocks []string
	var cur strings.Builder
	depthLi := 0
	flush := func(into *[]string) {
		if t := strings.TrimSpace(space.ReplaceAllString(cur.String(), " ")); t != "" {
			*into = append(*into, t)
		}
		cur.Reset()
	}
	for {
		switch z.Next() {
		case html.ErrorToken:
			flush(&blocks)
			if len(items) > 0 {
				return capItems(items)
			}
			return capItems(blocks)
		case html.TextToken:
			cur.Write(z.Text())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "li":
				flush(&blocks)
				depthLi++
			case "br", "p", "div", "h1", "h2", "h3", "h4", "ul", "ol":
				if depthLi == 0 {
					flush(&blocks)
				}
			case "script", "style":
				z.Next() // skip their text
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "li":
				flush(&items)
				depthLi--
			case "p", "div", "h1", "h2", "h3", "h4":
				if depthLi == 0 {
					flush(&blocks)
				}
			}
		}
	}
}

func capItems(in []string) []string {
	out := make([]string, 0, min(len(in), maxItems))
	for _, s := range in {
		if s == "" {
			continue
		}
		if r := []rune(s); len(r) > maxItemText {
			s = string(r[:maxItemText-1]) + "…"
		}
		out = append(out, s)
		if len(out) == maxItems {
			break
		}
	}
	return out
}
