package mojang

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// DefaultLibraryRepo serves libraries that name no URL of their own.
const DefaultLibraryRepo = "https://libraries.minecraft.net/"

// Library is one entry of a version's library list.
type Library struct {
	Name      string            `json:"name"`
	Downloads *LibraryDownloads `json:"downloads,omitempty"`
	URL       string            `json:"url,omitempty"` // maven repository base (loader profiles)
	// Mirrors are more repository bases tried in order after URL (legacy Forge
	// libraries, see the loader package). Not part of Mojang's format.
	Mirrors []string          `json:"mirrors,omitempty"`
	SHA1    string            `json:"sha1,omitempty"` // some loader profiles put it here
	Size    int64             `json:"size,omitempty"`
	Natives map[string]string `json:"natives,omitempty"` // os → classifier (legacy natives)
	Extract *Extract          `json:"extract,omitempty"`
	Rules   []Rule            `json:"rules,omitempty"`
}

// LibraryDownloads holds the main jar and native classifier jars.
type LibraryDownloads struct {
	Artifact    *Artifact           `json:"artifact,omitempty"`
	Classifiers map[string]Artifact `json:"classifiers,omitempty"`
}

// Extract lists path prefixes to skip when unpacking a natives jar.
type Extract struct {
	Exclude []string `json:"exclude,omitempty"`
}

// ResolvedLibrary is a library file the launch needs on this platform.
type ResolvedLibrary struct {
	Name    string   // maven coordinate, for logs
	Path    string   // slash-separated path inside the libraries folder
	URL     string   // empty when the file must already exist locally
	Mirrors []string // more full URLs for the same file, tried in order
	SHA1    string   // may be empty for loader libraries without one
	Size    int64
	Native  bool     // a legacy natives jar to extract, not a classpath entry
	Exclude []string // for natives
}

// ErrBadLibrary is returned for a library whose coordinate or path is unsafe.
var ErrBadLibrary = errors.New("mojang: invalid library")

var (
	mavenPart   = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
	pathSegment = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
	sha1Hex     = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// MavenPath turns "group:artifact:version[:classifier][@ext]" into its
// repository path, validating every part.
func MavenPath(coord string) (string, error) {
	ext := "jar"
	if i := strings.LastIndexByte(coord, '@'); i >= 0 {
		coord, ext = coord[:i], coord[i+1:]
	}
	parts := strings.Split(coord, ":")
	if len(parts) < 3 || len(parts) > 4 {
		return "", fmt.Errorf("%w: coordinate %q", ErrBadLibrary, coord)
	}
	for _, p := range append(parts, ext) {
		if !mavenPart.MatchString(p) || p == "." || p == ".." {
			return "", fmt.Errorf("%w: coordinate %q", ErrBadLibrary, coord)
		}
	}
	group, artifact, version := parts[0], parts[1], parts[2]
	for _, g := range strings.Split(group, ".") {
		if g == "" {
			return "", fmt.Errorf("%w: coordinate %q", ErrBadLibrary, coord)
		}
	}
	file := artifact + "-" + version
	if len(parts) == 4 {
		file += "-" + parts[3]
	}
	return strings.ReplaceAll(group, ".", "/") + "/" + artifact + "/" + version + "/" + file + "." + ext, nil
}

// ValidPath reports whether p is a safe, slash-separated relative path.
func ValidPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." || !pathSegment.MatchString(seg) {
			return false
		}
	}
	return true
}

// dedupKey identifies a library regardless of version: group:artifact[:classifier].
func dedupKey(coord string, native bool) string {
	coord, _, _ = strings.Cut(coord, "@")
	parts := strings.Split(coord, ":")
	key := coord
	if len(parts) >= 3 {
		key = parts[0] + ":" + parts[1]
		if len(parts) == 4 {
			key += ":" + parts[3]
		}
	}
	if native {
		key += "#natives"
	}
	return key
}

// ResolveLibraries returns the files needed on env, first entry winning for
// duplicates (so a merged loader profile's own versions take precedence).
func (v *Version) ResolveLibraries(env Env) ([]ResolvedLibrary, error) {
	return v.resolve(env, func(r ResolvedLibrary) string { return dedupKey(r.Name, r.Native) })
}

// ResolveFiles is ResolveLibraries without version de-duplication: every
// listed file, once per path. A loader installer's tools need exactly the
// versions they list, even when the game uses another.
func (v *Version) ResolveFiles(env Env) ([]ResolvedLibrary, error) {
	return v.resolve(env, func(r ResolvedLibrary) string { return r.Path })
}

func (v *Version) resolve(env Env, keyOf func(ResolvedLibrary) string) ([]ResolvedLibrary, error) {
	var out []ResolvedLibrary
	seen := map[string]bool{}
	add := func(r ResolvedLibrary) error {
		if !ValidPath(r.Path) {
			return fmt.Errorf("%w: path %q", ErrBadLibrary, r.Path)
		}
		if r.SHA1 != "" && !sha1Hex.MatchString(r.SHA1) {
			return fmt.Errorf("%w: checksum for %s", ErrBadLibrary, r.Name)
		}
		key := keyOf(r)
		if seen[key] {
			return nil
		}
		seen[key] = true
		out = append(out, r)
		return nil
	}
	for _, lib := range v.Libraries {
		if !Allowed(lib.Rules, env) {
			continue
		}
		if lib.Natives != nil {
			if r, ok, err := lib.native(env); err != nil {
				return nil, err
			} else if ok {
				if err := add(r); err != nil {
					return nil, err
				}
			}
		}
		r, ok, err := lib.artifact()
		if err != nil {
			return nil, err
		}
		if ok {
			if err := add(r); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// native resolves a legacy natives classifier for env.
func (lib Library) native(env Env) (ResolvedLibrary, bool, error) {
	classifier, ok := lib.Natives[env.OS]
	if !ok {
		return ResolvedLibrary{}, false, nil
	}
	bits := "64"
	if env.Arch == "x86" {
		bits = "32"
	}
	classifier = strings.ReplaceAll(classifier, "${arch}", bits)
	r := ResolvedLibrary{Name: lib.Name + ":" + classifier, Native: true}
	if lib.Extract != nil {
		r.Exclude = lib.Extract.Exclude
	}
	if lib.Downloads != nil {
		if a, ok := lib.Downloads.Classifiers[classifier]; ok {
			r.Path, r.URL, r.SHA1, r.Size = a.Path, a.URL, a.SHA1, a.Size
			return r, true, nil
		}
	}
	p, err := MavenPath(lib.Name + ":" + classifier)
	if err != nil {
		return r, false, err
	}
	r.Path, r.URL = p, repo(lib.URL)+p
	return r, true, nil
}

// artifact resolves the classpath jar, if the library has one.
func (lib Library) artifact() (ResolvedLibrary, bool, error) {
	r := ResolvedLibrary{Name: lib.Name}
	if lib.Downloads != nil {
		a := lib.Downloads.Artifact
		if a == nil {
			return r, false, nil // a natives-only legacy entry
		}
		r.Path, r.URL, r.SHA1, r.Size = a.Path, a.URL, a.SHA1, a.Size
		if r.Path == "" {
			p, err := MavenPath(lib.Name)
			if err != nil {
				return r, false, err
			}
			r.Path = p
		}
		return r, true, nil
	}
	if lib.Natives != nil {
		return r, false, nil
	}
	p, err := MavenPath(lib.Name)
	if err != nil {
		return r, false, err
	}
	r.Path, r.URL, r.SHA1, r.Size = p, repo(lib.URL)+p, lib.SHA1, lib.Size
	for _, m := range lib.Mirrors {
		r.Mirrors = append(r.Mirrors, repo(m)+p)
	}
	return r, true, nil
}

func repo(base string) string {
	if base == "" {
		return DefaultLibraryRepo
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base
}
