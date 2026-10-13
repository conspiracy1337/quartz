package exporting

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"

	"quartz/internal/content"
)

// listRow is one enabled mod in a mod list.
type listRow struct{ name, version, link, authors string }

func (s *Service) listRows(ctx context.Context, id string) ([]listRow, error) {
	items, err := s.d.Content.Manifest(ctx, id)
	if err != nil {
		return nil, err
	}
	var rows []listRow
	for _, it := range items {
		if it.Kind != content.Mods || !it.Enabled {
			continue
		}
		rows = append(rows, listRow{name: it.Name, version: it.Version, link: httpsOnly(it.Page), authors: strings.Join(it.Authors, ", ")})
	}
	if len(rows) == 0 {
		return nil, ErrNoMods
	}
	slices.SortFunc(rows, func(a, b listRow) int { return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)) })
	return rows, nil
}

func (s *Service) writeList(ctx context.Context, w io.Writer, id string, f ListFormat) error {
	rows, err := s.listRows(ctx, id)
	if err != nil {
		return err
	}
	if f == CSV {
		return writeCSV(w, rows)
	}
	return writeMarkdown(w, rows)
}

func writeCSV(w io.Writer, rows []listRow) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Name", "Version", "Link", "Authors"})
	for _, r := range rows {
		_ = cw.Write([]string{cell(r.name), cell(r.version), cell(r.link), cell(r.authors)})
	}
	cw.Flush()
	return cw.Error()
}

// cell keeps a spreadsheet from reading a value as a formula (CSV
// injection): a leading = + - @ tab or carriage return gets a quote mark.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func writeMarkdown(w io.Writer, rows []listRow) error {
	var b strings.Builder
	b.WriteString("| Mod | Version | Authors |\n| --- | --- | --- |\n")
	for _, r := range rows {
		name := md(r.name)
		if r.link != "" {
			name = "[" + name + "](" + strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(r.link) + ")"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", name, md(r.version), md(r.authors))
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

// httpsOnly keeps a project link only when it is a plain https address.
func httpsOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	return raw
}
