// Package packwiz reads and writes packwiz .pw.toml metadata files, the
// format Prism keeps beside mods, so instances stay interoperable. Both
// update sources are read and written, both lock keys (packwiz's pin and
// Prism's x-prismlauncher-lock-update) are honoured, and keys this package
// doesn't know survive a rewrite.
package packwiz

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"quartz/internal/core/fsx"
	"quartz/internal/core/tomlx"
)

// Ext is the metadata file extension.
const Ext = ".pw.toml"

// Limits for reading a metadata folder someone else may have written.
const (
	maxFile  = 1 << 20
	maxFiles = 10000
)

// ErrBadSlug is returned for slugs that can't be a file name.
var ErrBadSlug = errors.New("packwiz: invalid slug")

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// ValidSlug reports whether s is safe as a metadata file name.
func ValidSlug(s string) bool { return slugRe.MatchString(s) && !strings.Contains(s, "..") }

// Download is the [download] table.
type Download struct {
	Mode       string // url | metadata:curseforge
	URL        string
	HashFormat string // sha1 | sha512 | murmur2
	Hash       string
}

// Modrinth is [update.modrinth].
type Modrinth struct{ ModID, Version string }

// CurseForge is [update.curseforge].
type CurseForge struct{ ProjectID, FileID int64 }

// Dep is one x-prismlauncher-dependencies entry.
type Dep struct{ AddonID, Type, Version string }

// Meta is one .pw.toml file.
type Meta struct {
	Slug          string
	Name          string
	Filename      string
	Side          string
	Pin           bool
	Download      Download
	Modrinth      *Modrinth
	CurseForge    *CurseForge
	Loaders       []string
	MCVersions    []string
	ReleaseType   string
	VersionNumber string
	Deps          []Dep
	raw           map[string]any
}

// Read parses one metadata file.
func Read(path string) (Meta, error) {
	data, err := fsx.ReadCapped(path, maxFile)
	if err != nil {
		return Meta{}, err
	}
	doc, err := tomlx.Decode(data)
	if err != nil {
		return Meta{}, err
	}
	m := Meta{raw: doc, Slug: strings.TrimSuffix(filepath.Base(path), Ext)}
	m.Name, m.Filename, m.Side = str(doc, "name"), str(doc, "filename"), str(doc, "side")
	m.Pin = boolean(doc, "pin") || boolean(doc, "x-prismlauncher-lock-update")
	m.Loaders, m.MCVersions = strs(doc, "x-prismlauncher-loaders"), strs(doc, "x-prismlauncher-mc-versions")
	m.ReleaseType, m.VersionNumber = str(doc, "x-prismlauncher-release-type"), str(doc, "x-prismlauncher-version-number")
	if dl, ok := doc["download"].(map[string]any); ok {
		m.Download = Download{Mode: str(dl, "mode"), URL: str(dl, "url"), HashFormat: str(dl, "hash-format"), Hash: str(dl, "hash")}
	}
	if up, ok := doc["update"].(map[string]any); ok {
		if mr, ok := up["modrinth"].(map[string]any); ok {
			m.Modrinth = &Modrinth{ModID: str(mr, "mod-id"), Version: str(mr, "version")}
		}
		if cf, ok := up["curseforge"].(map[string]any); ok {
			m.CurseForge = &CurseForge{ProjectID: integer(cf, "project-id"), FileID: integer(cf, "file-id")}
		}
	}
	if deps, ok := doc["x-prismlauncher-dependencies"].([]any); ok {
		for _, d := range deps {
			if t, ok := d.(map[string]any); ok {
				m.Deps = append(m.Deps, Dep{AddonID: str(t, "addonId"), Type: str(t, "type"), Version: str(t, "version")})
			}
		}
	}
	return m, nil
}

// ReadDir reads every metadata file in dir, keyed by lower-case filename (the
// file they describe), so lookups ignore case as Prism's do.
func ReadDir(dir string) (map[string]Meta, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Meta{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]Meta{}
	n := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), Ext) {
			continue
		}
		if n++; n > maxFiles {
			break
		}
		m, err := Read(filepath.Join(dir, e.Name()))
		if err != nil || m.Filename == "" {
			continue
		}
		out[strings.ToLower(m.Filename)] = m
	}
	return out, nil
}

// Write stores m as dir/<slug>.pw.toml atomically. A file for the same slug
// differing only in case is replaced.
func Write(dir string, m Meta) error {
	if !ValidSlug(m.Slug) {
		return ErrBadSlug
	}
	doc := map[string]any{}
	for k, v := range m.raw {
		doc[k] = v
	}
	set := func(t map[string]any, k string, v any) {
		switch x := v.(type) {
		case string:
			if x == "" {
				delete(t, k)
				return
			}
		case []string:
			if len(x) == 0 {
				delete(t, k)
				return
			}
		}
		t[k] = v
	}
	set(doc, "name", m.Name)
	set(doc, "filename", m.Filename)
	set(doc, "side", orDefault(m.Side, "both"))
	set(doc, "x-prismlauncher-loaders", m.Loaders)
	set(doc, "x-prismlauncher-mc-versions", m.MCVersions)
	set(doc, "x-prismlauncher-release-type", m.ReleaseType)
	set(doc, "x-prismlauncher-version-number", m.VersionNumber)
	if m.Pin {
		doc["pin"], doc["x-prismlauncher-lock-update"] = true, true
	} else {
		delete(doc, "pin")
		delete(doc, "x-prismlauncher-lock-update")
	}
	deps := make([]map[string]any, 0, len(m.Deps))
	for _, d := range m.Deps {
		t := map[string]any{"addonId": d.AddonID, "type": d.Type}
		set(t, "version", d.Version)
		deps = append(deps, t)
	}
	if len(deps) > 0 {
		doc["x-prismlauncher-dependencies"] = deps
	} else {
		delete(doc, "x-prismlauncher-dependencies")
	}
	dl, _ := doc["download"].(map[string]any)
	if dl == nil {
		dl = map[string]any{}
	}
	set(dl, "mode", m.Download.Mode)
	set(dl, "url", m.Download.URL)
	set(dl, "hash-format", m.Download.HashFormat)
	set(dl, "hash", m.Download.Hash)
	doc["download"] = dl
	up, _ := doc["update"].(map[string]any)
	if up == nil {
		up = map[string]any{}
	}
	if m.Modrinth != nil {
		up["modrinth"] = map[string]any{"mod-id": m.Modrinth.ModID, "version": m.Modrinth.Version}
	} else {
		delete(up, "modrinth")
	}
	if m.CurseForge != nil {
		up["curseforge"] = map[string]any{"project-id": m.CurseForge.ProjectID, "file-id": m.CurseForge.FileID}
	} else {
		delete(up, "curseforge")
	}
	doc["update"] = up
	data, err := tomlx.Encode(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	removeOtherCase(dir, m.Slug)
	return fsx.AtomicWrite(filepath.Join(dir, m.Slug+Ext), data, fsx.FilePerm)
}

// Remove deletes the metadata for slug, matching its file name in any case.
func Remove(dir, slug string) error {
	if !ValidSlug(slug) {
		return ErrBadSlug
	}
	removeOtherCase(dir, slug)
	err := os.Remove(filepath.Join(dir, slug+Ext))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func removeOtherCase(dir, slug string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != slug+Ext && strings.EqualFold(e.Name(), slug+Ext) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// Slug derives a metadata name from a project slug or file name.
func Slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if b.Len() >= 64 {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == '.':
			if !strings.HasSuffix(b.String(), ".") {
				b.WriteRune(r)
			}
		case b.Len() > 0:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-._")
	if !ValidSlug(out) {
		return "item"
	}
	return out
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func str(t map[string]any, k string) string {
	s, _ := t[k].(string)
	return s
}

func boolean(t map[string]any, k string) bool {
	b, _ := t[k].(bool)
	return b
}

func integer(t map[string]any, k string) int64 {
	switch v := t[k].(type) {
	case int64:
		return v
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func strs(t map[string]any, k string) []string {
	list, _ := t[k].([]any)
	var out []string
	for _, v := range list {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
