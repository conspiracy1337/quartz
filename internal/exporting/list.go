package exporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/url"
	"slices"
	"strings"

	"quartz/internal/content"
	"quartz/internal/instance"
	"quartz/internal/loader"
)

// ModList is a mod list as JSON: the file a JSON export writes and the
// document a code shares. Every mod in it can be downloaded again from the
// platform it names, so another launcher can rebuild the instance from it.
type ModList struct {
	Schema    int         `json:"schema"` // 1
	Name      string      `json:"name"`
	Minecraft string      `json:"minecraft"`
	Loader    *ListLoader `json:"loader,omitempty"`
	Mods      []ListMod   `json:"mods"`
}

// ListLoader is the mod loader a list runs on.
type ListLoader struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

// ListMod is one mod and where it downloads from.
type ListMod struct {
	Name    string   `json:"name"`
	Version string   `json:"version,omitempty"`
	Authors []string `json:"authors,omitempty"`
	Source  string   `json:"source"` // modrinth | curseforge
	Project string   `json:"project"`
	File    string   `json:"file"`
	URL     string   `json:"url"`
	SHA1    string   `json:"sha1,omitempty"`
	Page    string   `json:"page,omitempty"`
}

// maxListMods caps a list: no real instance comes near it, and a code's
// upload stays small.
const maxListMods = 2000

// listed is an instance's mod list and the mods left out of it: unlisted
// ones no platform serves, and turned-off ones.
type listed struct {
	list     ModList
	unlisted []content.ManifestItem
	off      []content.ManifestItem
}

// modList builds id's mod list, asking the platforms about files nobody
// identified yet so as few mods as possible are left out.
func (s *Service) modList(ctx context.Context, id string) (listed, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return listed{}, err
	}
	items, err := s.items(ctx, id)
	if err != nil {
		return listed{}, err
	}
	out := listed{list: ModList{Schema: 1, Name: in.Name, Minecraft: in.Minecraft, Loader: listLoader(in), Mods: []ListMod{}}}
	for _, it := range items {
		switch {
		case it.Kind != content.Mods:
		case !it.Enabled:
			out.off = append(out.off, it)
		default:
			if m, ok := listMod(it); ok && len(out.list.Mods) < maxListMods {
				out.list.Mods = append(out.list.Mods, m)
			} else {
				out.unlisted = append(out.unlisted, it)
			}
		}
	}
	if len(out.list.Mods) == 0 && len(out.unlisted) == 0 {
		return listed{}, ErrNoMods
	}
	slices.SortFunc(out.list.Mods, func(a, b ListMod) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

func listLoader(in instance.Instance) *ListLoader {
	k, err := loader.ParseKind(in.Loader)
	if err != nil || in.Loader == "" {
		return nil
	}
	return &ListLoader{Kind: in.Loader, Version: loader.Display(k, in.Minecraft, in.LoaderVersion)}
}

// listMod is a mod as a list names it: on Modrinth, else on CurseForge,
// with a download address. A mod neither serves can't be in a list.
func listMod(it content.ManifestItem) (ListMod, bool) {
	for _, id := range []*content.Ident{it.Modrinth, it.CurseForge} {
		if id == nil || httpsOnly(id.URL) == "" || id.ProjectID == "" || id.VersionID == "" {
			continue
		}
		return ListMod{Name: it.Name, Version: it.Version, Authors: it.Authors, Source: id.Source, Project: id.ProjectID, File: id.VersionID,
			URL: id.URL, SHA1: it.SHA1, Page: httpsOnly(it.Page)}, true
	}
	return ListMod{}, false
}

// render writes a list file: a Markdown table or the JSON document.
func render(w io.Writer, l ModList, f ListFormat) error {
	if f == JSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(l)
	}
	return writeMarkdown(w, l.Mods)
}

// renderedSize is how large a list file comes out.
func renderedSize(l ModList, f ListFormat) int64 {
	var buf bytes.Buffer
	_ = render(&buf, l, f)
	return int64(buf.Len())
}

func (s *Service) writeList(ctx context.Context, w io.Writer, id string, f ListFormat) error {
	l, err := s.modList(ctx, id)
	if err != nil {
		return err
	}
	if len(l.list.Mods) == 0 {
		return ErrNoMods
	}
	return render(w, l.list, f)
}

// writeHTML writes the list as an HTML list, every text escaped (the
// modlist.html a CurseForge zip carries).
func writeHTML(w io.Writer, mods []ListMod) error {
	var b strings.Builder
	b.WriteString("<ul>\n")
	for _, m := range mods {
		name := html.EscapeString(m.Name)
		if m.Page != "" {
			name = `<a href="` + html.EscapeString(m.Page) + `">` + name + `</a>`
		}
		b.WriteString("<li>" + name)
		if m.Version != "" {
			b.WriteString(" " + html.EscapeString(m.Version))
		}
		if len(m.Authors) > 0 {
			b.WriteString(" (by " + html.EscapeString(strings.Join(m.Authors, ", ")) + ")")
		}
		b.WriteString("</li>\n")
	}
	b.WriteString("</ul>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func writeMarkdown(w io.Writer, mods []ListMod) error {
	var b strings.Builder
	b.WriteString("| Mod | Version | Authors |\n| --- | --- | --- |\n")
	for _, m := range mods {
		name := md(m.Name)
		if m.Page != "" {
			name = "[" + name + "](" + strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(m.Page) + ")"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", name, md(m.Version), md(strings.Join(m.Authors, ", ")))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// md escapes text for a Markdown table cell: no markup, links or HTML from a
// mod's own metadata, and no line breaks.
var mdEscaper = strings.NewReplacer(
	`\`, `\\`, "|", `\|`, "[", `\[`, "]", `\]`, "<", "&lt;", ">", "&gt;", "*", `\*`, "_", `\_`, "`", "\\`",
	"\r", " ", "\n", " ",
)

func md(s string) string { return mdEscaper.Replace(s) }

// httpsOnly keeps a link only when it is a plain https address.
func httpsOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	return raw
}
