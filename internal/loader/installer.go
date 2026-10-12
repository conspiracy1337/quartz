package loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"quartz/internal/core/archive"
	"quartz/internal/core/download"
	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
	"quartz/internal/meta"
	"quartz/internal/mojang"
)

// installer reads Forge-style installer jars (Forge and NeoForge share the
// format). Preparing a build downloads its installer once, keeps its two
// profile files in versions/<kind>-<id>/, unpacks the libraries it embeds and
// the data its processors need; from then on the build is read from disk.
type installer struct {
	kind     Kind
	meta     *meta.Service
	dl       *download.Downloader
	verified *download.Verified
	dirs     Dirs
	locks    *locks
}

func newInstaller(d Deps, k Kind) installer {
	return installer{kind: k, meta: d.Meta, dl: d.Downloader, verified: d.Verified, dirs: d.Dirs, locks: &locks{}}
}

// Size caps for what an installer may contain.
const (
	maxProfileJSON = 4 << 20
	maxEmbedded    = 128 << 20
)

// The legacy Forge maven moved; its old addresses still appear in profiles.
var legacyForgeHosts = []string{"http://files.minecraftforge.net/maven/", "https://files.minecraftforge.net/maven/", "http://maven.minecraftforge.net/"}

const (
	mavenCentral = "https://repo1.maven.org/maven2/"
	preparedName = ".prepared"
)

// installProfile is install_profile.json in both its shapes: modern
// (minecraft, json, data, processors, libraries) and legacy (install, versionInfo).
type installProfile struct {
	Minecraft  string                             `json:"minecraft"`
	JSON       string                             `json:"json"`
	Data       map[string]struct{ Client string } `json:"data"`
	Processors []processor                        `json:"processors"`
	Libraries  []mojang.Library                   `json:"libraries"`

	Install     *legacyInstall  `json:"install"`
	VersionInfo json.RawMessage `json:"versionInfo"`
}

type processor struct {
	Sides     []string          `json:"sides"`
	Jar       string            `json:"jar"`
	Classpath []string          `json:"classpath"`
	Args      []string          `json:"args"`
	Outputs   map[string]string `json:"outputs"`
}

type legacyInstall struct {
	Path      string `json:"path"`     // maven coordinate of the universal jar
	FilePath  string `json:"filePath"` // its name inside the installer
	Minecraft string `json:"minecraft"`
}

func (in *installer) prepare(ctx context.Context, req Request, id, coord, repo string) (*Profile, error) {
	rel, err := mojang.MavenPath(coord)
	if err != nil {
		return nil, err
	}
	jar, err := fsx.SafeJoin(in.dirs.Libraries, filepath.FromSlash(rel))
	if err != nil {
		return nil, err
	}
	dir, err := fsx.SafeJoin(in.dirs.Versions, string(in.kind)+"-"+id)
	if err != nil {
		return nil, err
	}
	unlock := in.locks.lock(dir)
	defer unlock()

	stamp, _ := os.ReadFile(filepath.Join(dir, preparedName)) //nolint:gosec // inside the store
	if len(stamp) == 0 {
		if stamp, err = in.unpack(ctx, req, repo+rel, jar, dir); err != nil {
			return nil, err
		}
	}
	return in.profile(req, dir, jar, strings.TrimSpace(string(stamp)))
}

// unpack downloads and verifies the installer, then keeps what later
// launches need. It returns the installer's sha1, written last as the marker.
func (in *installer) unpack(ctx context.Context, req Request, url, jar, dir string) ([]byte, error) {
	// An installer is only trusted against the repository's own checksum.
	sum, err := in.meta.SidecarSHA1(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("loader: %s installer checksum: %w", in.kind.Name(), err)
	}
	if err := in.dl.Fetch(ctx, download.Item{URLs: []string{url}, Dest: jar, Sum: hashx.Of(hashx.SHA1, sum)}, nil); err != nil {
		return nil, fmt.Errorf("loader: %s installer: %w", in.kind.Name(), err)
	}
	raw, err := archive.ReadFile(jar, "install_profile.json", maxProfileJSON)
	if err != nil {
		return nil, err
	}
	var p installProfile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("loader: %s install profile: %w", in.kind.Name(), err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := fsx.AtomicWrite(filepath.Join(dir, "install_profile.json"), raw, fsx.FilePerm); err != nil {
		return nil, err
	}
	if p.Install != nil {
		err = in.unpackLegacy(jar, p.Install)
	} else {
		err = in.unpackModern(jar, dir, p)
	}
	if err != nil {
		return nil, err
	}
	return []byte(sum), fsx.AtomicWrite(filepath.Join(dir, preparedName), []byte(sum), fsx.FilePerm)
}

func (in *installer) unpackModern(jar, dir string, p installProfile) error {
	name := strings.TrimPrefix(p.JSON, "/")
	if name == "" {
		name = "version.json"
	}
	v, err := archive.ReadFile(jar, name, maxProfileJSON)
	if err != nil {
		return err
	}
	if err := fsx.AtomicWrite(filepath.Join(dir, "version.json"), v, fsx.FilePerm); err != nil {
		return err
	}
	// Libraries the installer carries itself (Forge's own jars on 1.16 and older).
	if err := archive.ExtractZip(jar, in.dirs.Libraries, archive.Options{Prefix: "maven/", MaxTotalSize: maxEmbedded}); err != nil {
		return err
	}
	// Data files the processors read, such as the binary patches.
	for _, d := range p.Data {
		if !strings.HasPrefix(d.Client, "/") {
			continue
		}
		name := strings.TrimPrefix(d.Client, "/")
		dst, err := fsx.SafeJoin(dir, filepath.FromSlash(name))
		if err != nil {
			return err
		}
		body, err := archive.ReadFile(jar, name, maxEmbedded)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if err := fsx.AtomicWrite(dst, body, fsx.FilePerm); err != nil {
			return err
		}
	}
	return nil
}

func (in *installer) unpackLegacy(jar string, inst *legacyInstall) error {
	rel, err := mojang.MavenPath(inst.Path)
	if err != nil {
		return err
	}
	dst, err := fsx.SafeJoin(in.dirs.Libraries, filepath.FromSlash(rel))
	if err != nil {
		return err
	}
	body, err := archive.ReadFile(jar, inst.FilePath, maxEmbedded)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return fsx.AtomicWrite(dst, body, fsx.FilePerm)
}

// profile builds the Profile from the files unpack kept.
func (in *installer) profile(req Request, dir, jar, stamp string) (*Profile, error) {
	raw, err := fsx.ReadCapped(filepath.Join(dir, "install_profile.json"), maxProfileJSON)
	if err != nil {
		return nil, err
	}
	var p installProfile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("loader: %s install profile: %w", in.kind.Name(), err)
	}
	if p.Install != nil {
		return in.legacyProfile(req, p)
	}
	if p.Minecraft != req.Minecraft {
		return nil, fmt.Errorf("loader: this %s build is for Minecraft %s, not %s", in.kind.Name(), p.Minecraft, req.Minecraft)
	}
	data, err := fsx.ReadCapped(filepath.Join(dir, "version.json"), maxProfileJSON)
	if err != nil {
		return nil, err
	}
	v, err := mojang.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("loader: %s version: %w", in.kind.Name(), err)
	}
	if err := inherits(v, req.Minecraft); err != nil {
		return nil, err
	}
	extra, err := (&mojang.Version{Libraries: p.Libraries}).ResolveFiles(req.Env)
	if err != nil {
		return nil, err
	}
	prof := &Profile{Version: v, Extra: extra, Produced: map[string]bool{}}
	vars := in.dataMap(req, dir, jar, p)
	proc, err := in.processing(req, vars, p.Processors, prof, filepath.Join(dir, ".processed"), stamp)
	if err != nil {
		return nil, err
	}
	if len(proc.steps) > 0 {
		prof.Processing = proc
		prof.NoClientJar = true
	}
	return prof, nil
}

// inherits checks a loader version JSON extends exactly mc (filling it in
// when the profile leaves it implicit).
func inherits(v *mojang.Version, mc string) error {
	switch v.InheritsFrom {
	case "":
		v.InheritsFrom = mc
	case mc:
	default:
		return fmt.Errorf("loader: profile %s extends %s, not %s", v.ID, v.InheritsFrom, mc)
	}
	return nil
}

// legacyLibrary is a library entry of a pre-1.13 Forge profile.
type legacyLibrary struct {
	Name      string            `json:"name"`
	URL       string            `json:"url"`
	ClientReq *bool             `json:"clientreq"`
	Natives   map[string]string `json:"natives"`
	Extract   *mojang.Extract   `json:"extract"`
	Rules     []mojang.Rule     `json:"rules"`
}

type legacyVersion struct {
	mojang.Version
	Libraries []legacyLibrary `json:"libraries"`
}

// legacyProfile converts versionInfo: libraries the client doesn't need are
// dropped, the universal jar points at the copy unpack placed, and the dead
// Forge maven addresses become the current one with Maven Central and
// Mojang's repository as fallbacks (Forge shipped some of these jars only as
// .pack.xz, which nothing in pure Go can unpack).
func (in *installer) legacyProfile(req Request, p installProfile) (*Profile, error) {
	if p.Install.Minecraft != req.Minecraft {
		return nil, fmt.Errorf("loader: this %s build is for Minecraft %s, not %s", in.kind.Name(), p.Install.Minecraft, req.Minecraft)
	}
	var lv legacyVersion
	if err := json.Unmarshal(p.VersionInfo, &lv); err != nil {
		return nil, fmt.Errorf("loader: %s version: %w", in.kind.Name(), err)
	}
	v := lv.Version
	v.Libraries = nil
	for _, l := range lv.Libraries {
		if l.ClientReq != nil && !*l.ClientReq {
			continue
		}
		lib := mojang.Library{Name: l.Name, Natives: l.Natives, Extract: l.Extract, Rules: l.Rules}
		switch repo := legacyRepo(l.URL); {
		case l.Name == p.Install.Path:
			rel, err := mojang.MavenPath(l.Name)
			if err != nil {
				return nil, err
			}
			lib.Downloads = &mojang.LibraryDownloads{Artifact: &mojang.Artifact{Path: rel}} // placed by unpack
		case repo == forgeRepo:
			lib.URL, lib.Mirrors = forgeRepo, []string{mavenCentral, mojang.DefaultLibraryRepo}
		case repo == "":
			lib.Mirrors = []string{mavenCentral, forgeRepo}
		default:
			lib.URL = repo
		}
		v.Libraries = append(v.Libraries, lib)
	}
	if err := inherits(&v, req.Minecraft); err != nil {
		return nil, err
	}
	return &Profile{Version: &v}, nil
}

// legacyRepo maps a legacy profile's repository to the one to use now ("" for
// Mojang's default).
func legacyRepo(u string) string {
	if u == "" {
		return ""
	}
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}
	for _, old := range legacyForgeHosts {
		if u == old {
			return forgeRepo
		}
	}
	return "https://" + strings.TrimPrefix(strings.TrimPrefix(u, "http://"), "https://")
}

// dataVars are what processor arguments refer to: the values every Forge
// installer defines (taken literally) and install_profile's data for the
// client side (resolved by expand when used, so a bad entry only fails if
// something needs it).
type dataVars struct {
	builtin map[string]string
	data    map[string]string
	dir     string // where unpack kept the installer's data files
}

func (in *installer) dataMap(req Request, dir, jar string, p installProfile) dataVars {
	v := dataVars{dir: dir, data: map[string]string{}, builtin: map[string]string{
		"SIDE":              "client",
		"MINECRAFT_JAR":     req.ClientJar,
		"MINECRAFT_VERSION": req.Minecraft,
		"ROOT":              filepath.Dir(in.dirs.Libraries),
		"INSTALLER":         jar,
		"LIBRARY_DIR":       in.dirs.Libraries,
	}}
	for k, d := range p.Data {
		v.data[k] = d.Client
	}
	return v
}

var dataKey = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// errUnknownKey is returned for a {KEY} the profile never defines; Pandora
// panicked here.
var errUnknownKey = errors.New("loader: installer refers to an undefined value")

// expand resolves one processor argument or data value:
//   - "[group:artifact:version:classifier@ext]" is a file in the libraries folder;
//   - "'text'" is the literal text;
//   - "/data/x" in a data entry is a file the installer carried (unpacked into dir);
//   - "{KEY}" occurrences are replaced by the built-in value, or the data
//     entry resolved the same way.
func (in *installer) expand(vars dataVars, s string, depth int) (string, error) {
	if depth > 4 {
		return "", fmt.Errorf("%w: %q refers to itself", errUnknownKey, s)
	}
	switch {
	case len(s) > 2 && s[0] == '[' && s[len(s)-1] == ']':
		return in.libraryPath(s[1 : len(s)-1])
	case len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'':
		return s[1 : len(s)-1], nil
	case strings.HasPrefix(s, "/") && depth > 0:
		return fsx.SafeJoin(vars.dir, filepath.FromSlash(strings.TrimPrefix(s, "/")))
	}
	var err error
	out := dataKey.ReplaceAllStringFunc(s, func(m string) string {
		key := m[1 : len(m)-1]
		if v, ok := vars.builtin[key]; ok {
			return v
		}
		v, ok := vars.data[key]
		if !ok {
			err = fmt.Errorf("%w: {%s}", errUnknownKey, key)
			return ""
		}
		r, e := in.expand(vars, v, depth+1)
		if e != nil {
			err = e
		}
		return r
	})
	return out, err
}

func (in *installer) libraryPath(coord string) (string, error) {
	rel, err := mojang.MavenPath(coord)
	if err != nil {
		return "", err
	}
	return fsx.SafeJoin(in.dirs.Libraries, filepath.FromSlash(rel))
}
