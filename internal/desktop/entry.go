package desktop

import (
	"strings"
)

// Entry is a freedesktop.org desktop entry (an app-menu item, a desktop
// shortcut or an autostart item).
type Entry struct {
	Name       string
	Comment    string
	Exec       []string // the program and its arguments, quoted by Marshal
	ExecSuffix string   // a field code such as %U, appended unquoted
	Icon       string   // an icon name or an absolute path
	Categories []string
	MimeTypes  []string
	NoDisplay  bool
	Extra      []string // extra "Key=value" lines, already valid
}

// Marshal writes the entry. Every value comes from us or from an instance
// name, so values are cleaned of line breaks and escaped per the spec.
func (e Entry) Marshal() []byte {
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			b.WriteString(k + "=" + v + "\n")
		}
	}
	b.WriteString("[Desktop Entry]\nType=Application\nVersion=1.5\n")
	line("Name", escapeString(e.Name))
	line("Comment", escapeString(e.Comment))
	exec := ExecLine(e.Exec)
	if e.ExecSuffix != "" {
		exec += " " + e.ExecSuffix
	}
	line("Exec", exec)
	line("Icon", escapeString(e.Icon))
	line("Terminal", "false")
	if len(e.Categories) > 0 {
		line("Categories", strings.Join(e.Categories, ";")+";")
	}
	if len(e.MimeTypes) > 0 {
		line("MimeType", strings.Join(e.MimeTypes, ";")+";")
	}
	if e.NoDisplay {
		line("NoDisplay", "true")
	}
	for _, x := range e.Extra {
		b.WriteString(x + "\n")
	}
	return []byte(b.String())
}

// escapeString applies the spec's string escapes and drops control
// characters, so a value can't start a new key.
func escapeString(v string) string {
	v = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, v)
	return strings.ReplaceAll(v, `\`, `\\`)
}

// ExecLine quotes each argument for an Exec key: inside double quotes, `"`,
// "`", `$` and `\` are backslash-escaped, then the string escapes double every
// backslash again; `%` is written `%%` so it is never a field code.
func ExecLine(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		a = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, a)
		var q strings.Builder
		for _, r := range a {
			switch r {
			case '"', '`', '$', '\\':
				q.WriteRune('\\')
			}
			q.WriteRune(r)
		}
		quoted := `"` + q.String() + `"`
		quoted = strings.ReplaceAll(quoted, `\`, `\\`)
		parts[i] = strings.ReplaceAll(quoted, "%", "%%")
	}
	return strings.Join(parts, " ")
}
