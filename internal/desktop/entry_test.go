package desktop

import (
	"strings"
	"testing"
)

func TestExecLine(t *testing.T) {
	cases := map[string][]string{
		`"/opt/quartz"`:                             {"/opt/quartz"},
		`"/home/a b/quartz" "--launch" "x"`:         {"/home/a b/quartz", "--launch", "x"},
		`"/p/q" "100%%"`:                            {"/p/q", "100%"},
		`"/p/q" "a\\"b" "\\$HOME" "\\` + "`" + `x"`: {"/p/q", `a"b`, "$HOME", "`x"},
		`"/p/q" "C:\\\\x"`:                          {"/p/q", `C:\x`},
		`"/p/q" "ab"`:                               {"/p/q", "a\nb"},
	}
	for want, argv := range cases {
		if got := ExecLine(argv); got != want {
			t.Errorf("%q:\n got %s\nwant %s", argv, got, want)
		}
	}
}

func TestEntryMarshal(t *testing.T) {
	e := Entry{Name: "My\nPack", Exec: []string{"/opt/quartz", "--launch", "abc"}, ExecSuffix: "%U", Icon: "/i/icon.png",
		Categories: []string{"Game"}, MimeTypes: []string{"x-scheme-handler/modrinth"}}
	got := string(e.Marshal())
	want := `[Desktop Entry]
Type=Application
Version=1.5
Name=MyPack
Exec="/opt/quartz" "--launch" "abc" %U
Icon=/i/icon.png
Terminal=false
Categories=Game;
MimeType=x-scheme-handler/modrinth;
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Count(got, "\n") != strings.Count(want, "\n") {
		t.Fatal("a value must never add a line")
	}
}
