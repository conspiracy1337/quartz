// Package links reads what the launcher is asked to open from outside: links
// from web pages (modrinth://, curseforge://, our own scheme), https links
// and pack files. Links come from any web page or local program, so only a
// short allowlist of shapes is accepted, and none of them installs anything
// by itself: each one at most shows the Import preview or a project page.
package links

import (
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"quartz/internal/brand"
	"quartz/internal/instance"
	"quartz/internal/intent"
)

// Kind is what a link opens.
type Kind int

const (
	PackFile      Kind = iota + 1 // a .mrpack or .zip on disk: the Import preview
	PackLink                      // an https link: the Import preview
	ModrinthPack                  // modrinth://modpack/<id>: the Import preview
	ModrinthPage                  // modrinth://mod/<id> (and other types): the project page
	CurseForge                    // curseforge://install: a pack's preview or a project page
	OpenInstance                  // quartz://open/<id>: the instance's page
)

// Link is a parsed link.
type Link struct {
	Kind    Kind
	Path    string // PackFile: an absolute path
	URL     string // PackLink
	Project string // a Modrinth id or slug, or a CurseForge project id
	File    string // CurseForge: the file id
	ID      string // OpenInstance
}

// ErrUnsupported means the link isn't one Quartz opens.
var ErrUnsupported = errors.New("links: that link can't be opened in Quartz")

const maxLink = 2048

var (
	slugRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	digitsRe = regexp.MustCompile(`^[0-9]{1,12}$`)
	// The project types modrinth.com's install buttons use.
	modrinthPages = map[string]bool{"mod": true, "resourcepack": true, "shader": true, "datapack": true, "plugin": true}
)

// Parse reads raw: a link, or an absolute file path (intent makes command
// line paths absolute).
func Parse(raw string) (Link, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxLink*2 {
		return Link{}, ErrUnsupported
	}
	if !intent.IsLink(raw) {
		return parseFile(raw)
	}
	if len(raw) > maxLink {
		return Link{}, ErrUnsupported
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return Link{}, ErrUnsupported
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		if u.Host == "" {
			return Link{}, ErrUnsupported
		}
		return Link{Kind: PackLink, URL: raw}, nil
	case "modrinth":
		return parseModrinth(u)
	case "curseforge":
		return parseCurseForge(u)
	case brand.Scheme:
		return parseOwn(u)
	}
	return Link{}, ErrUnsupported
}

func parseFile(p string) (Link, error) {
	if !filepath.IsAbs(p) {
		return Link{}, ErrUnsupported
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mrpack", ".zip":
		return Link{Kind: PackFile, Path: filepath.Clean(p)}, nil
	}
	return Link{}, ErrUnsupported
}

// segments returns a URL's host and path parts: in modrinth://mod/abc the
// "host" is the type.
func segments(u *url.URL) []string {
	parts := []string{u.Host}
	for _, s := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return parts
}

// parseModrinth reads modrinth://<type>/<id or slug>.
func parseModrinth(u *url.URL) (Link, error) {
	p := segments(u)
	if len(p) != 2 || !slugRe.MatchString(p[1]) {
		return Link{}, ErrUnsupported
	}
	switch {
	case p[0] == "modpack":
		return Link{Kind: ModrinthPack, Project: p[1]}, nil
	case modrinthPages[p[0]]:
		return Link{Kind: ModrinthPage, Project: p[1]}, nil
	}
	return Link{}, ErrUnsupported
}

// parseCurseForge reads curseforge://install?addonId=N&fileId=M.
func parseCurseForge(u *url.URL) (Link, error) {
	p := segments(u)
	if len(p) != 1 || p[0] != "install" {
		return Link{}, ErrUnsupported
	}
	q := u.Query()
	addon, file := q.Get("addonId"), q.Get("fileId")
	if !digitsRe.MatchString(addon) || (file != "" && !digitsRe.MatchString(file)) {
		return Link{}, ErrUnsupported
	}
	return Link{Kind: CurseForge, Project: addon, File: file}, nil
}

// parseOwn reads quartz://import?url=<https link> and quartz://open/<id>.
// There is deliberately no way to start a game from a link.
func parseOwn(u *url.URL) (Link, error) {
	p := segments(u)
	switch {
	case len(p) == 1 && p[0] == "import":
		target := u.Query().Get("url")
		inner, err := url.Parse(target)
		if err != nil || inner.Scheme != "https" || inner.Host == "" || inner.User != nil || len(target) > maxLink {
			return Link{}, ErrUnsupported
		}
		return Link{Kind: PackLink, URL: target}, nil
	case len(p) == 2 && p[0] == "open" && instance.ValidID(p[1]):
		return Link{Kind: OpenInstance, ID: p[1]}, nil
	}
	return Link{}, ErrUnsupported
}
