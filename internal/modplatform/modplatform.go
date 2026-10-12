// Package modplatform talks to Modrinth and CurseForge through one Provider
// interface: identify files by hash, look projects up, and find the newest
// version of what an instance has. The update algorithm is ported from
// Prism (ModrinthCheckUpdate, FlameCheckUpdate): one loader per request,
// then a forced pass for loaders the files themselves declare.
package modplatform

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Source names a platform.
type Source string

const (
	Modrinth   Source = "modrinth"
	CurseForge Source = "curseforge"
)

// Channel is how unstable an update may be.
type Channel int

const (
	Release Channel = iota
	Beta
	Alpha
)

// ParseChannel reads the settings value (release | beta | alpha).
func ParseChannel(s string) Channel {
	switch s {
	case "beta":
		return Beta
	case "alpha":
		return Alpha
	}
	return Release
}

// Types lists the release types a channel allows.
func (c Channel) Types() []string {
	return []string{"release", "beta", "alpha"}[:c+1]
}

// Allows reports whether a version of release type t may be offered.
func (c Channel) Allows(t string) bool {
	for _, v := range c.Types() {
		if v == t {
			return true
		}
	}
	return false
}

// DepKind is how a version relates to another project.
type DepKind string

const (
	DepRequired     DepKind = "required"
	DepOptional     DepKind = "optional"
	DepIncompatible DepKind = "incompatible"
	DepEmbedded     DepKind = "embedded"
)

// Dep is one dependency of a version.
type Dep struct {
	ProjectID string  `json:"projectId"`
	VersionID string  `json:"versionId,omitempty"`
	Kind      DepKind `json:"kind"`
}

// File is the downloadable file of a version.
type File struct {
	Name   string `json:"name"`
	URL    string `json:"url"` // "" when the author blocks third-party downloads (CurseForge)
	Size   int64  `json:"size"`
	SHA1   string `json:"sha1"`
	SHA512 string `json:"sha512,omitempty"`
	// Page is where a person can download a blocked file by hand.
	Page string `json:"page,omitempty"`
}

// Version is one release of a project.
type Version struct {
	Source    Source    `json:"source"`
	ProjectID string    `json:"projectId"`
	ID        string    `json:"id"`
	Number    string    `json:"number"`
	Type      string    `json:"type"` // release | beta | alpha
	Published time.Time `json:"published"`
	Loaders   []string  `json:"loaders"`
	Game      []string  `json:"game"`
	File      File      `json:"file"`
	Deps      []Dep     `json:"deps,omitempty"`
	// Class is the project's type, when the answer says (CurseForge Files).
	Class ProjectType `json:"class,omitempty"`
	// Changelog is filled only by Changelogs (Markdown, or HTML on CurseForge).
	Changelog string `json:"-"`
}

// Project is a platform's page for a mod or pack.
type Project struct {
	Source  Source `json:"source"`
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Library bool   `json:"library"`
	Icon    string `json:"icon,omitempty"` // picture URL on the platform CDN
	Summary string `json:"summary,omitempty"`
}

// Hashes identify one file.
type Hashes struct {
	SHA1, SHA512 string
	Murmur2      uint32
}

// Query narrows versions: game versions, loaders, and the channel.
type Query struct {
	Game    []string
	Loaders []string
	Channel Channel
}

// Provider is one platform.
type Provider interface {
	Source() Source
	// Ready is false when the platform can't be used (CurseForge without a key).
	Ready() bool
	// Identify finds the versions files belong to, keyed by sha1.
	Identify(ctx context.Context, files []Hashes) (map[string]Version, error)
	Projects(ctx context.Context, ids []string) (map[string]Project, error)
	// Latest finds the newest allowed version for each item, for one loader
	// ("" for none), keyed by Installed.Key.
	Latest(ctx context.Context, items []Installed, loader string, q Query) (map[string]Version, error)
	// Versions lists a project's versions, newest first, without changelogs.
	Versions(ctx context.Context, project string, q Query) ([]Version, error)
	// Changelogs fills in the changelog of each version.
	Changelogs(ctx context.Context, vs []Version) error
}

// Doer is the HTTP client (netx.Client).
type Doer interface {
	JSON(ctx context.Context, method, rawURL string, h http.Header, in, out any) error
}

// Installed is one identified item an update check looks at.
type Installed struct {
	Key       string // the caller's id for the item
	ProjectID string
	Hashes    Hashes
	Current   Version
	Loaders   []string // loaders the file itself declares
}

// Check finds updates for every installed item on one platform: the newest
// allowed version, when it is newer than the installed one and another file.
func Check(ctx context.Context, p Provider, items []Installed, q Query) (map[string]Version, error) {
	found, err := Find(ctx, p, items, q)
	for key, v := range found {
		if it := itemByKey(items, key); !newer(v, it.Current) {
			delete(found, key)
		}
	}
	return found, err
}

func itemByKey(items []Installed, key string) Installed {
	for _, it := range items {
		if it.Key == key {
			return it
		}
	}
	return Installed{}
}

// Find finds the newest allowed version of every installed item on one
// platform, even when that is the installed one (to see what supports a
// game version). It asks one loader at a time in the order given, then once
// more for loaders the files declare that the instance doesn't (Prism's
// forced pass); items answered drop out of later requests.
func Find(ctx context.Context, p Provider, items []Installed, q Query) (map[string]Version, error) {
	chain := append([]string{}, q.Loaders...)
	if len(chain) == 0 {
		chain = []string{""}
	}
	forced := map[string]bool{}
	for _, it := range items {
		for _, l := range it.Loaders {
			if !contains(chain, l) {
				forced[l] = true
			}
		}
	}
	extra := make([]string, 0, len(forced))
	for l := range forced {
		extra = append(extra, l)
	}
	sort.Strings(extra)
	out := map[string]Version{}
	answered := map[string]bool{}
	var errs []error
	ask := func(loader string, only func(Installed) bool) {
		var batch []Installed
		for _, it := range items {
			if !answered[it.Key] && only(it) {
				batch = append(batch, it)
			}
		}
		if len(batch) == 0 {
			return
		}
		latest, err := p.Latest(ctx, batch, loader, q)
		if err != nil {
			errs = append(errs, err)
		}
		for _, it := range batch {
			v, ok := latest[it.Key]
			if !ok || !q.Channel.Allows(v.Type) {
				continue
			}
			answered[it.Key] = true
			out[it.Key] = v
		}
	}
	for _, l := range chain {
		ask(l, func(Installed) bool { return true })
	}
	for _, l := range extra {
		ask(l, func(it Installed) bool { return contains(it.Loaders, l) })
	}
	return out, errors.Join(errs...)
}

// newer reports whether v replaces the installed version: another file,
// published later (so a newer beta is never "updated" back to a release).
func newer(v, cur Version) bool {
	if v.ID == cur.ID || (v.File.SHA1 != "" && v.File.SHA1 == cur.File.SHA1) {
		return false
	}
	return cur.Published.IsZero() || v.Published.After(cur.Published)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Chunk splits ids into batches of at most n.
func Chunk[T any](ids []T, n int) [][]T {
	var out [][]T
	for len(ids) > n {
		out = append(out, ids[:n])
		ids = ids[n:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)

// ValidID reports whether s can be a project or version id on either
// platform (Modrinth: base62; CurseForge: digits).
func ValidID(s string) bool { return idRe.MatchString(s) }

// ValidURL reports whether a download URL is https on a platform CDN.
func ValidURL(raw string) bool {
	for _, prefix := range []string{"https://cdn.modrinth.com/", "https://edge.forgecdn.net/", "https://mediafilez.forgecdn.net/"} {
		if strings.HasPrefix(raw, prefix) && !strings.ContainsAny(raw, " \r\n\\") {
			return true
		}
	}
	return false
}
