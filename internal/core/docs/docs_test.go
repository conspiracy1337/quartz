package docs

import (
	"encoding/json"
	"strings"
	"testing"
)

func pic(u string) string {
	if strings.HasPrefix(u, "https://cdn.modrinth.com/") {
		return "/tex/remote/x.png"
	}
	return ""
}

func dump(b []Block) string {
	out, _ := json.Marshal(b)
	return string(out)
}

func TestMarkdown(t *testing.T) {
	src := "# Sodium\n\n![banner](https://cdn.modrinth.com/data/x/banner.png)\n\nSodium is a **free** and [open-source](https://github.com/CaffeineMC/sodium) " +
		"rendering engine. Needs `Fabric API`.\n\n## Features\n\n- Faster *chunks*\n- Fewer stutters\n  - nested\n\n" +
		"```\ncode block\n```\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n<center><img src=\"https://shields.io/badge.svg\"> <b>Raw</b> html</center>\n\n" +
		"[bad](javascript:alert(1)) <script>alert(1)</script>"
	got := dump(Markdown(src, pic))
	want := `[{"type":"h2","spans":[{"text":"Sodium"}]},{"type":"img","src":"/tex/remote/x.png","alt":"banner"},` +
		`{"type":"p","spans":[{"text":"Sodium is a free and "},{"text":"open-source","href":"https://github.com/CaffeineMC/sodium"},` +
		`{"text":" rendering engine. Needs "},{"text":"Fabric API","code":true},{"text":"."}]},` +
		`{"type":"h2","spans":[{"text":"Features"}]},{"type":"ul","items":[[{"text":"Faster chunks"}],[{"text":"Fewer stutters"}],[{"text":"nested"}]]},` +
		`{"type":"p","spans":[{"text":"code block","code":true}]},{"type":"p","spans":[{"text":"a · b"}]},{"type":"p","spans":[{"text":"1 · 2"}]},` +
		`{"type":"p","spans":[{"text":"Raw html"}]},{"type":"p","spans":[{"text":"bad"}]}]`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestHTMLFromCurseForge(t *testing.T) {
	src := `<p><span style="font-size: 18px;"><strong>Jade</strong></span></p><p>Shows what you&nbsp;look at.<br>Second line</p>` +
		`<ul><li><p>One</p></li><li>Two <a href="http://insecure.example">link</a></li></ul><iframe src="https://youtube.com"></iframe>` +
		`<p><img src="https://cdn.modrinth.com/a.png" alt="Shot"></p>`
	got := dump(HTML(src, pic))
	want := `[{"type":"p","spans":[{"text":"Jade"}]},{"type":"p","spans":[{"text":"Shows what you look at."}]},{"type":"p","spans":[{"text":"Second line"}]},` +
		`{"type":"ul","items":[[{"text":"One"}],[{"text":"Two link"}]]},{"type":"img","src":"/tex/remote/x.png","alt":"Shot"}]`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestCaps(t *testing.T) {
	src := strings.Repeat("Paragraph.\n\n", 1000)
	if n := len(Markdown(src, nil)); n != maxBlocks {
		t.Fatalf("%d blocks", n)
	}
	long := strings.Repeat("word ", 20_000)
	b := Markdown(long+"\n\nafter", nil)
	if len(b) != 1 {
		t.Fatalf("text past the character cap kept: %d blocks", len(b))
	}
	if HTML("", nil) != nil {
		t.Fatal("empty input")
	}
}
