package docs

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// Markdown used goldmark until internal/core/markdown replaced it for its
// size. These tests hold the two to the same blocks, which is all a
// description shows. goldmark is a test-only dependency now.

var goldmarkMD = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// goldmarkBlocks is Markdown as it was with goldmark.
func goldmarkBlocks(src string, pic Picture) []Block {
	if len(src) > maxInput {
		src = src[:maxInput]
	}
	var buf bytes.Buffer
	if err := goldmarkMD.Convert([]byte(src), &buf); err != nil {
		return nil
	}
	return HTML(buf.String(), pic)
}

func keepPictures(u string) string { return u }

// MatchesGoldmark reports where Markdown and goldmark disagree on src.
func matchesGoldmark(src string) (got, want string, ok bool) {
	got, want = dump(Markdown(src, keepPictures)), dump(goldmarkBlocks(src, keepPictures))
	return got, want, got == want
}

const testdata = "../markdown/testdata/"

// corpus is every Markdown input the tests know: the CommonMark spec,
// goldmark's own cases, the cases below and this package's other tests.
func corpus(t testing.TB) []string {
	t.Helper()
	b, err := os.ReadFile(testdata + "spec.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec []struct {
		Markdown string `json:"markdown"`
	}
	if err := json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ex := range spec {
		out = append(out, ex.Markdown)
	}
	for _, f := range []string{"extra", "linkify", "strikethrough", "table", "tasklist"} {
		b, err := os.ReadFile(testdata + "goldmark-" + f + ".txt")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range strings.Split(string(b), "//= = = = = = = = = = = = = = = = = = = = = = = =//") {
			if parts := strings.Split(c, "//- - - - - - - - -//\n"); len(parts) >= 3 {
				out = append(out, parts[1])
			}
		}
	}
	return append(out, descriptionCases...)
}

// descriptionCases are shapes common in Modrinth descriptions.
var descriptionCases = []string{
	"<p align=\"center\">\n  <img src=\"https://cdn.modrinth.com/a.png\" width=\"600\">\n</p>\n\n# Title\n",
	"<center>\n\n![banner](https://cdn.modrinth.com/b.png)\n\n</center>\n\n**Bold** text",
	"[![Discord](https://img.shields.io/discord/1?logo=discord)](https://discord.gg/x) [![Ko-fi](https://ko-fi.com/img.png)](https://ko-fi.com/y)",
	"<details>\n<summary>Spoiler</summary>\n\n- one\n- two\n\n</details>",
	"| Feature | Status |\n|:--|:-:|\n| A | ✅ |\n| B | ❌ |\nAfter",
	"Intro\n| a | b |\n| - | - |\n| 1 | 2 |\n---\n",
	"Visit https://modrinth.com/mod/sodium, or www.curseforge.com/minecraft. Mail me@example.com!",
	"(see https://example.com/a_(b)_c) and ~https://x.com~ and **https://y.com**",
	"- [x] done\n- [ ] todo\n  - [ ] nested\n\n1. one\n2) two",
	"Line one  \nline two\\\nline three",
	"<br>\n<img src=\"https://cdn.modrinth.com/c.png\" alt=\"c\"/><br/>\ntext <b>bold</b> &amp; &copy; &#169; &#x1F600; &notanentity;",
	"```java\npublic class A {}\n```\n\n    indented\n\n~~~\ntilde\n~~~",
	"> quote with **bold**\n> > nested\n\n***\n___\n- - -",
	"[ref link][ref] and [Ref] and [ref][]\n\n[ref]: https://example.com/ref \"Title\"",
	"![alt *em* `code`](https://cdn.modrinth.com/i.png \"t\") ![ref img][i]\n\n[i]: <https://cdn.modrinth.com/j k.png>",
	"Setext\n===\n\nSetext 2\n---",
	"<!-- comment -->\nText <!-- inline --> after <span style=\"color:red\">red</span>",
	"\tTabbed\n  \t* item\n\t\t* nested",
	"Emoji :smile: and unicode ünïcödé — “quotes” ‘single’",
	"<https://autolink.example/x> <mailto:a@b.c> <a@b.co>",
	"# Title\r\n\r\n- a\r\n- b\r\n  - c\r\n\r\n| a | b |\r\n|---|:-:|\r\n| 1 | 2 |\r\n\r\n```\r\ncode\r\n```\r\n",
	"-\r\n- \r\n*\r\n1.\r\ntext  \r\nbreak\\\r\nend\r\n",
	"- a\n  -\n\n- b\n  more\n\n1. a\n2.\n\n3. c\n   d\n\n* x\n*     000\n    *\n\n  0",
}

func TestMarkdownMatchesGoldmark(t *testing.T) {
	for _, src := range corpus(t) {
		if got, want, ok := matchesGoldmark(src); !ok {
			t.Errorf("%q\n got %s\nwant %s", src, got, want)
		}
	}
}

// reachable narrows fuzz input to what descriptions hold and the two
// parsers are meant to agree on. Descriptions come from JSON, so they are
// valid UTF-8, and they break lines with \n (\r\n is rare; goldmark keeps
// the \r in a few places, which the CRLF cases below cover). Tabs become
// spaces: goldmark measures a tab inside a list marker's padding
// differently from CommonMark (and this package); the spec examples and
// the corpus cover tabs.
func reachable(src string) string {
	src = strings.ToValidUTF8(src, "\uFFFD")
	var b strings.Builder
	for i := 0; i < len(src); i++ {
		switch {
		case src[i] == '\r':
		case src[i] == '\t':
			b.WriteString("    ")
		case src[i] == '\v' || src[i] == '\f':
			b.WriteByte(' ') // never in descriptions; goldmark trims them in places
		default:
			b.WriteByte(src[i])
		}
	}
	return b.String()
}

func FuzzMarkdownMatchesGoldmark(f *testing.F) {
	for _, src := range corpus(f) {
		f.Add(src)
	}
	f.Fuzz(func(t *testing.T, src string) {
		src = reachable(src)
		if strings.Count(src, ">")+strings.Count(src, "*")+strings.Count(src, "-")+strings.Count(src, "+")+strings.Count(src, ".")+strings.Count(src, ")") >= 64 {
			return // could nest past the parser's depth cap, where goldmark has none
		}
		if got, want, ok := matchesGoldmark(src); !ok {
			t.Errorf("%q\n got %s\nwant %s", src, got, want)
		}
	})
}
