// Package install prepares a Minecraft version for launch (XMCL's resolve →
// diagnose → plan → execute): it resolves the version, its libraries, assets,
// log configuration and Java, drops whatever is already in place and verified,
// and plans the rest as one download job. A fully installed version plans
// nothing, so launching it touches no network and shows no download.
package install

import (
	"context"
	"crypto/sha1" //nolint:gosec // names a folder of natives, not security
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"

	"quartz/internal/core/archive"
	"quartz/internal/core/download"
	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
	"quartz/internal/core/linkx"
	"quartz/internal/core/netx"
	"quartz/internal/core/tasks"
	"quartz/internal/java"
	"quartz/internal/loader"
	"quartz/internal/meta"
	"quartz/internal/mojang"
)

// Dirs are the shared store folders.
type Dirs struct {
	Libraries  string
	Versions   string
	Assets     string
	LogConfigs string
	Natives    string
}

// Installer is safe for concurrent use.
type Installer struct {
	meta    *meta.Service
	dl      *download.Downloader
	java    *java.Manager
	loaders *loader.Registry
	dirs    Dirs
}

// New returns an installer.
func New(m *meta.Service, dl *download.Downloader, j *java.Manager, l *loader.Registry, d Dirs) *Installer {
	return &Installer{meta: m, dl: dl, java: j, loaders: l, dirs: d}
}

// Target is what one instance plays: a Minecraft version and, optionally, a
// mod loader build on top of it.
type Target struct {
	Minecraft     string
	Loader        loader.Kind // "" for vanilla
	LoaderVersion string
}

// Resolved is everything a launch needs to know about a version.
type Resolved struct {
	Version      *mojang.Version // vanilla, or the loader profile merged over it
	Minecraft    string
	Env          mojang.Env
	LibrariesDir string
	Index        mojang.AssetIndex
	indexData    []byte
	Libraries    []mojang.ResolvedLibrary
	ClientJar    string
	Classpath    []string // libraries and the client jar, absolute
	NativesDir   string
	AssetsRoot   string
	GameAssets   string // the folder legacy versions read assets from by name
	LogConfig    string // absolute path, "" when the version has none
	LogArgument  string // e.g. -Dlog4j.configurationFile=${path}

	// From the loader, if any: files only the install needs, library paths
	// its processors create, and the processing itself.
	Extra      []mojang.ResolvedLibrary
	Produced   map[string]bool
	Processing *loader.Processing
}

var (
	versionID = regexp.MustCompile(`^[A-Za-z0-9._ +-]{1,64}$`)
	fileID    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
)

// Resolve reads the target's metadata (from cache when possible) and works out
// every file path. gameDir is needed for map_to_resources assets. A loader's
// profile is merged over vanilla; the client jar, natives and assets stay
// vanilla's, so every loader on one Minecraft version shares them.
func (in *Installer) Resolve(ctx context.Context, t Target, gameDir string, env mojang.Env) (*Resolved, error) {
	id := t.Minecraft
	if !versionID.MatchString(id) {
		return nil, fmt.Errorf("install: invalid version id %q", id)
	}
	parent, err := in.meta.Version(ctx, id)
	if err != nil {
		return nil, err
	}
	r := &Resolved{Version: parent, Minecraft: id, Env: env, AssetsRoot: in.dirs.Assets, LibrariesDir: in.dirs.Libraries}
	if r.ClientJar, err = fsx.SafeJoin(in.dirs.Versions, filepath.Join(id, id+".jar")); err != nil {
		return nil, err
	}
	var prof *loader.Profile
	if t.Loader != "" {
		prof, err = in.loaders.Profile(ctx, t.Loader, loader.Request{Minecraft: id, Env: env, Parent: parent, ClientJar: r.ClientJar}, t.LoaderVersion)
		if err != nil {
			return nil, err
		}
		r.Version = mojang.Merge(parent, prof.Version)
		r.Extra, r.Produced, r.Processing = prof.Extra, prof.Produced, prof.Processing
	}
	v := r.Version
	if r.Libraries, err = v.ResolveLibraries(env); err != nil {
		return nil, err
	}
	in.fillChecksums(ctx, r.Libraries)
	in.fillChecksums(ctx, r.Extra)
	if r.Index, r.indexData, err = in.meta.AssetIndex(ctx, v); err != nil {
		return nil, err
	}
	var natives []string
	for _, l := range r.Libraries {
		p, err := fsx.SafeJoin(in.dirs.Libraries, l.Path)
		if err != nil {
			return nil, err
		}
		if l.Native {
			natives = append(natives, l.SHA1+l.Path)
			continue
		}
		r.Classpath = append(r.Classpath, p)
	}
	if prof == nil || !prof.NoClientJar {
		r.Classpath = append(r.Classpath, r.ClientJar)
	}
	r.NativesDir = filepath.Join(in.dirs.Natives, nativesKey(id, natives))
	switch {
	case r.Index.MapToResources:
		r.GameAssets = filepath.Join(gameDir, "resources")
	case r.Index.Virtual:
		r.GameAssets = filepath.Join(in.dirs.Assets, "virtual", sanitize(v.AssetsID()))
	default:
		r.GameAssets = in.dirs.Assets
	}
	r.LogConfig, r.LogArgument = in.logConfig(v)
	return r, nil
}

// logConfig is where a version's log4j configuration goes and the argument
// naming it ("" when the version has none).
func (in *Installer) logConfig(v *mojang.Version) (path, arg string) {
	if lc := v.Logging; lc != nil && lc.Client != nil && fileID.MatchString(lc.Client.File.ID) {
		return filepath.Join(in.dirs.LogConfigs, lc.Client.File.ID), lc.Client.Argument
	}
	return "", ""
}

// fillChecksums asks the maven repository for the sha1 of libraries whose
// metadata names none (common in loader profiles), so they are verified like
// everything else. The answers are cached for good; a repository without
// checksums leaves the library unhashed (see download.Verified).
func (in *Installer) fillChecksums(ctx context.Context, libs []mojang.ResolvedLibrary) {
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range libs {
		l := &libs[i]
		if l.SHA1 != "" || l.URL == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if sum, err := in.meta.SidecarSHA1(ctx, l.URL); err == nil {
				l.SHA1 = sum
			}
		}()
	}
	wg.Wait()
}

// nativesKey names the natives folder after the exact set of native jars, so
// versions sharing natives share one folder and a change gets a fresh one.
func nativesKey(id string, natives []string) string {
	slices.Sort(natives)
	h := sha1.New() //nolint:gosec // naming only
	_, _ = io.WriteString(h, id+"\x00"+runtime.GOOS+runtime.GOARCH)
	for _, n := range natives {
		_, _ = io.WriteString(h, "\x00"+n)
	}
	return sanitize(id) + "-" + hex.EncodeToString(h.Sum(nil))[:12]
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, s)
}

// files lists every download the version needs (before dropping ready ones).
func (in *Installer) files(r *Resolved) (game, assets []download.Item, err error) {
	v := r.Version
	client, ok := v.Downloads["client"]
	if !ok {
		return nil, nil, fmt.Errorf("install: %s has no client download", v.ID)
	}
	game = append(game, item([]string{client.URL}, r.ClientJar, client.Size, client.SHA1))
	libs, err := in.libraryItems(append(slices.Clone(r.Libraries), r.Extra...), r.Produced)
	if err != nil {
		return nil, nil, err
	}
	game = append(game, libs...)
	if r.LogConfig != "" {
		f := v.Logging.Client.File
		game = append(game, item([]string{f.URL}, r.LogConfig, f.Size, f.SHA1))
	}
	seen := map[string]bool{}
	for _, o := range r.Index.Objects {
		if !o.Valid() || seen[o.Hash] {
			continue
		}
		seen[o.Hash] = true
		dest := filepath.Join(in.dirs.Assets, "objects", filepath.FromSlash(o.ObjectPath()))
		assets = append(assets, item([]string{o.URL()}, dest, o.Size, o.Hash))
	}
	return game, assets, nil
}

// libraryItems are the downloads of libraries; one with no address must be
// in place already (unpacked from a loader installer) or be produced by the
// loader's processors.
func (in *Installer) libraryItems(libs []mojang.ResolvedLibrary, produced map[string]bool) ([]download.Item, error) {
	var out []download.Item
	for _, l := range libs {
		dest, err := fsx.SafeJoin(in.dirs.Libraries, l.Path)
		if err != nil {
			return nil, err
		}
		if l.URL == "" {
			if _, err := os.Stat(dest); err != nil && !produced[l.Path] {
				return nil, fmt.Errorf("install: library %s is missing and has no download", l.Name)
			}
			continue
		}
		out = append(out, item(append([]string{l.URL}, l.Mirrors...), dest, l.Size, l.SHA1))
	}
	return out, nil
}

func item(urls []string, dest string, size int64, sha1 string) download.Item {
	it := download.Item{URLs: urls, Dest: dest, Size: size}
	if sha1 != "" {
		it.Sum = hashx.Of(hashx.SHA1, sha1)
	}
	return it
}

// Request is one install for one launch.
type Request struct {
	Resolved *Resolved
	Meta     tasks.Meta
	Java     *java.Plan // set when the Java runtime must be installed too
	Force    bool       // Repair: verify every file again by hashing
	// ProcessorJava finds the Java a loader's processors run with, once the
	// job's own Java step (if any) is done. Needed only when Processing is set.
	ProcessorJava func(context.Context) (string, error)
}

// Plan builds the job, or returns nil when everything is already in place.
func (in *Installer) Plan(req Request) (*tasks.Spec, error) {
	r := req.Resolved
	game, assets, err := in.files(r)
	if err != nil {
		return nil, err
	}
	if req.Force {
		for _, it := range append(slices.Clone(game), assets...) {
			in.dl.Forget(it)
		}
	}
	if err := in.writeIndex(r); err != nil {
		return nil, err
	}
	spec := tasks.NewJob(req.Meta)
	var pending bool
	if works := in.pending(game); len(works) > 0 {
		spec.Step(fmt.Sprintf("Minecraft %s and libraries", r.Minecraft), tasks.Bytes, works...)
		pending = true
	}
	if works := in.pending(assets); len(works) > 0 {
		spec.Step("Assets", tasks.Count, works...)
		pending = true
	}
	if req.Java != nil {
		req.Java.AddTo(spec)
		pending = true
	}
	if req.Force || !complete(r.NativesDir) {
		spec.After("Natives", tasks.Count, tasks.Func("extract natives", func(context.Context) error { return in.extractNatives(r) }))
		pending = true
	}
	if r.Index.Legacy() && (req.Force || !complete(r.GameAssets)) {
		spec.After("Legacy assets", tasks.Count, tasks.Func("copy legacy assets", func(context.Context) error { return in.legacyAssets(r) }))
		pending = true
	}
	added, err := addProcessing(spec, r.Processing, req.Force, req.ProcessorJava)
	if err != nil {
		return nil, err
	}
	if !pending && !added {
		return nil, nil
	}
	return spec, nil
}

// addProcessing adds a loader's processing to a job unless it already ran.
func addProcessing(spec *tasks.Spec, p *loader.Processing, force bool, javaFor func(context.Context) (string, error)) (bool, error) {
	if p == nil || (!force && p.Done()) {
		return false, nil
	}
	if javaFor == nil {
		return false, errors.New("install: this loader needs Java to finish installing")
	}
	spec.After("Processing Minecraft", tasks.Count, tasks.Func("run the loader's installer", func(ctx context.Context) error {
		javaPath, err := javaFor(ctx)
		if err == nil {
			err = p.Run(ctx, javaPath)
		}
		if err != nil && ctx.Err() == nil {
			// A processor fails the same way every time; retrying only delays the message.
			return &netx.PermanentError{Err: err}
		}
		return err
	}))
	return true, nil
}

// Estimate is how many bytes the first launch of a version still downloads.
func (in *Installer) Estimate(r *Resolved) (int64, error) {
	game, assets, err := in.files(r)
	if err != nil {
		return 0, err
	}
	var n int64
	for _, it := range append(game, assets...) {
		if !in.dl.Ready(it) {
			n += it.Total()
		}
	}
	return n, nil
}

func (in *Installer) pending(items []download.Item) []tasks.Work {
	var works []tasks.Work
	for _, it := range items {
		if !in.dl.Ready(it) {
			works = append(works, tasks.Download(it))
		}
	}
	return works
}

// writeIndex stores the asset index where the game reads it.
func (in *Installer) writeIndex(r *Resolved) error {
	path := filepath.Join(in.dirs.Assets, "indexes", sanitize(r.Version.AssetsID())+".json")
	if cur, err := fsx.ReadCapped(path, 64<<20); err == nil && string(cur) == string(r.indexData) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fsx.AtomicWrite(path, r.indexData, fsx.FilePerm)
}

const completeMarker = ".complete"

func complete(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, completeMarker))
	return err == nil
}

func markComplete(dir string) error {
	return fsx.AtomicWrite(filepath.Join(dir, completeMarker), nil, fsx.FilePerm)
}

// extractNatives unpacks legacy natives jars (honouring extract.exclude) into
// a fresh folder, then swaps it into place.
func (in *Installer) extractNatives(r *Resolved) error {
	tmp := r.NativesDir + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	for _, l := range r.Libraries {
		if !l.Native {
			continue
		}
		jar, err := fsx.SafeJoin(in.dirs.Libraries, l.Path)
		if err != nil {
			return err
		}
		exclude := l.Exclude
		err = archive.ExtractZip(jar, tmp, archive.Options{MaxTotalSize: 256 << 20, Include: func(name string) bool {
			for _, ex := range exclude {
				if strings.HasPrefix(name, ex) {
					return false
				}
			}
			return true
		}})
		if err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("extracting %s: %w", filepath.Base(jar), err)
		}
	}
	if err := markComplete(tmp); err != nil {
		return err
	}
	_ = os.RemoveAll(r.NativesDir)
	return os.Rename(tmp, r.NativesDir)
}

// legacyAssets lays assets out by name for versions that read them that way
// (Pandora got this wrong and wrote hashed paths there).
func (in *Installer) legacyAssets(r *Resolved) error {
	for name, o := range r.Index.Objects {
		if !o.Valid() || !mojang.ValidAssetName(name) {
			continue
		}
		dst, err := fsx.SafeJoin(r.GameAssets, name)
		if err != nil {
			continue
		}
		src := filepath.Join(in.dirs.Assets, "objects", filepath.FromSlash(o.ObjectPath()))
		if info, err := os.Stat(dst); err == nil && info.Size() == o.Size {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if _, err := linkx.Place(src, dst, linkx.Options{Perm: fsx.FilePerm}); err != nil {
			return err
		}
	}
	return markComplete(r.GameAssets)
}
