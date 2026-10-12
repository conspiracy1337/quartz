package java

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"quartz/internal/core/archive"
	"quartz/internal/core/download"
	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
	"quartz/internal/core/httpcache"
	"quartz/internal/core/tasks"
	"quartz/internal/mojang"
)

// errNoBuild means a provider publishes nothing for this major and platform;
// the next provider is tried.
var errNoBuild = errors.New("java: provider has no build for this platform")

var vendorNames = map[string]string{"temurin": "Eclipse Temurin", "zulu": "Azul Zulu", "mojang": "Mojang"}

// VendorName is the display name of a vendor setting value.
func VendorName(v string) string { return vendorNames[v] }

// Plan installs one runtime: its downloads, then a setup step that unpacks
// or links files and writes the completion marker.
type Plan struct {
	Major     int
	Vendor    string
	Downloads []tasks.Work
	Setup     tasks.Work
}

// AddTo appends the plan's steps to a job.
func (p *Plan) AddTo(s *tasks.Spec) *tasks.Spec {
	return s.Step(fmt.Sprintf("Java %d runtime", p.Major), tasks.Bytes, p.Downloads...).
		After(fmt.Sprintf("Setting up Java %d", p.Major), tasks.Count, p.Setup)
}

// Plan finds a provider that has Java major for this machine, starting with
// the vendor from the settings and falling back to the others.
func (m *Manager) Plan(ctx context.Context, major int, component string) (*Plan, error) {
	order := []string{"temurin", "zulu", "mojang"}
	if v := m.cfg.Vendor(); v != "" {
		order = append([]string{v}, slicesWithout(order, v)...)
	}
	var errs []error
	for _, vendor := range order {
		var (
			p   *Plan
			err error
		)
		switch vendor {
		case "mojang":
			p, err = m.planMojang(ctx, major, component)
		default:
			p, err = m.planArchive(ctx, vendor, major)
		}
		if err == nil {
			return p, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", vendor, err))
		if ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("%w (Java %d): %w", ErrNoRuntime, major, errors.Join(errs...))
}

func slicesWithout(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// mojangComponents maps a Java major to Mojang's runtime component.
var mojangComponents = map[int]string{8: "jre-legacy", 16: "java-runtime-alpha", 17: "java-runtime-gamma", 21: "java-runtime-delta", 25: "java-runtime-epsilon"}

var componentName = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

func (m *Manager) planMojang(ctx context.Context, major int, component string) (*Plan, error) {
	platform := mojang.RuntimePlatform(runtime.GOOS, runtime.GOARCH)
	if component == "" {
		component = mojangComponents[major]
	}
	if platform == "" || !componentName.MatchString(component) {
		return nil, errNoBuild
	}
	idx, err := m.cfg.Meta.RuntimeIndex(ctx)
	if err != nil {
		return nil, err
	}
	build, ok := idx.Build(platform, component)
	if !ok {
		return nil, errNoBuild
	}
	man, err := m.cfg.Meta.RuntimeManifest(ctx, build)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(m.cfg.Dir, "mojang-"+component+"-"+platform)
	p := &Plan{Major: major, Vendor: "mojang"}
	var dirs []string
	links := map[string]string{}
	for name, f := range man.Files {
		dest, err := fsx.SafeJoin(dir, name)
		if err != nil {
			return nil, err
		}
		switch f.Type {
		case "directory":
			dirs = append(dirs, dest)
		case "link":
			links[dest] = f.Target
		case "file":
			it, err := runtimeItem(dest, f)
			if err != nil {
				return nil, err
			}
			p.Downloads = append(p.Downloads, tasks.Download(it))
		}
	}
	version := build.Version.Name
	p.Setup = tasks.Func("set up Java", func(context.Context) error {
		for _, d := range dirs {
			if err := os.MkdirAll(d, 0o700); err != nil {
				return err
			}
		}
		for link, target := range links {
			if err := placeLink(dir, link, target); err != nil {
				return err
			}
		}
		if err := checkLinks(dir, links); err != nil {
			return err
		}
		return finishInstall(dir, "mojang", major, version)
	})
	return p, nil
}

// runtimeItem turns one Mojang runtime file into a download, preferring the
// lzma copy (about a third of the size) and verifying the decoded bytes.
func runtimeItem(dest string, f mojang.RuntimeFile) (download.Item, error) {
	raw, ok := f.Downloads["raw"]
	if !ok {
		return download.Item{}, fmt.Errorf("java: runtime file %s has no download", filepath.Base(dest))
	}
	it := download.Item{URLs: []string{raw.URL}, Dest: dest, Size: raw.Size, Sum: hashx.Of(hashx.SHA1, raw.SHA1), Exec: f.Executable}
	if lz, ok := f.Downloads["lzma"]; ok {
		it.URLs, it.Size, it.Sum = []string{lz.URL}, lz.Size, hashx.Of(hashx.SHA1, lz.SHA1)
		it.Decode = download.DecodeLZMA
		it.Raw = &download.Check{Size: raw.Size, Sum: hashx.Of(hashx.SHA1, raw.SHA1)}
	}
	return it, nil
}

// placeLink creates a runtime link whose target must stay inside root; where
// symlinks are not allowed (Windows without privilege) the file is copied.
func placeLink(root, link, target string) error {
	resolved := filepath.Join(filepath.Dir(link), filepath.FromSlash(target))
	if _, err := fsx.SafeJoin(root, mustRel(root, resolved)); err != nil || filepath.IsAbs(target) {
		return fmt.Errorf("java: runtime link %s leaves its folder", filepath.Base(link))
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		return err
	}
	_ = os.Remove(link)
	if err := os.Symlink(filepath.FromSlash(target), link); err == nil {
		return nil
	}
	data, err := os.ReadFile(resolved) //nolint:gosec // resolved was checked to lie inside root
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(link, data, 0o700)
}

// checkLinks re-checks every created link against the real file system, so a
// chain of links cannot lead outside the runtime folder.
func checkLinks(root string, links map[string]string) error {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	for link := range links {
		if !fsx.SymlinkInside(real, link) {
			for l := range links {
				_ = os.Remove(l)
			}
			return fmt.Errorf("java: runtime link %s leaves its folder", filepath.Base(link))
		}
	}
	return nil
}

func mustRel(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return ".."
	}
	return rel
}

// finishInstall finds the java binary and writes the completion marker.
func finishInstall(dir, vendor string, major int, version string) error {
	bins := findBinaries(dir, exe(runtime.GOOS), 5)
	if len(bins) == 0 {
		return fmt.Errorf("%w: no java binary in the %s download", ErrNotJava, vendorNames[vendor])
	}
	best := bins[0]
	for _, b := range bins { // prefer the shallowest (bin/java over jre/bin/java)
		if strings.Count(b, string(filepath.Separator)) < strings.Count(best, string(filepath.Separator)) {
			best = b
		}
	}
	rel, err := filepath.Rel(dir, best)
	if err != nil {
		return err
	}
	return writeMarker(dir, marker{Vendor: vendor, Major: major, Version: version, Java: filepath.ToSlash(rel)})
}

// archiveRelease is what the vendor APIs tell us about one JRE archive.
type archiveRelease struct {
	url, name, version string
	size, estimate     int64
	sum                hashx.Sum
}

var archiveFile = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,200}$`)

// planArchive installs a JRE archive from Eclipse Temurin or Azul Zulu.
func (m *Manager) planArchive(ctx context.Context, vendor string, major int) (*Plan, error) {
	var (
		rel archiveRelease
		err error
	)
	switch vendor {
	case "temurin":
		rel, err = m.adoptium(ctx, major)
	case "zulu":
		rel, err = m.zulu(ctx, major)
	default:
		return nil, errNoBuild
	}
	if err != nil {
		return nil, err
	}
	if !archiveFile.MatchString(rel.name) {
		rel.name = fmt.Sprintf("%s-%d%s", vendor, major, archiveExt())
	}
	archivePath := filepath.Join(m.cfg.Dir, ".archives", rel.name)
	dir := filepath.Join(m.cfg.Dir, fmt.Sprintf("%s-%d-%s-%s", vendor, major, runtime.GOOS, runtime.GOARCH))
	it := download.Item{URLs: []string{rel.url}, Dest: archivePath, Size: rel.size, Estimate: rel.estimate, Sum: rel.sum}
	p := &Plan{Major: major, Vendor: vendor, Downloads: []tasks.Work{tasks.Download(it)}}
	p.Setup = tasks.Func("unpack Java", func(context.Context) error {
		tmp := dir + ".tmp"
		_ = os.RemoveAll(tmp)
		opts := archive.Options{MaxTotalSize: 2 << 30}
		var err error
		if strings.HasSuffix(rel.name, ".zip") {
			err = archive.ExtractZip(archivePath, tmp, opts)
		} else {
			err = archive.ExtractTarGz(archivePath, tmp, opts)
		}
		if err == nil {
			err = finishInstall(tmp, vendor, major, rel.version)
		}
		if err == nil {
			_ = os.RemoveAll(dir)
			err = os.Rename(tmp, dir)
		}
		if err != nil {
			_ = os.RemoveAll(tmp)
			return err
		}
		_ = os.Remove(archivePath)
		return nil
	})
	return p, nil
}

func archiveExt() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

const vendorAPITTL = time.Hour

func (m *Manager) adoptium(ctx context.Context, major int) (archiveRelease, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "aarch64", "386": "x86"}[runtime.GOARCH]
	osName := map[string]string{"linux": "linux", "darwin": "mac", "windows": "windows"}[runtime.GOOS]
	if arch == "" || osName == "" {
		return archiveRelease{}, errNoBuild
	}
	u := fmt.Sprintf("https://api.adoptium.net/v3/assets/latest/%d/hotspot?architecture=%s&image_type=jre&os=%s&vendor=eclipse", major, arch, osName)
	type asset struct {
		Binary struct {
			Package struct {
				Link     string `json:"link"`
				Name     string `json:"name"`
				Checksum string `json:"checksum"`
				Size     int64  `json:"size"`
			} `json:"package"`
		} `json:"binary"`
		Version struct {
			Semver string `json:"semver"`
		} `json:"version"`
	}
	list, err := httpcache.GetJSON[[]asset](ctx, m.cfg.Cache, u, httpcache.Policy{TTL: vendorAPITTL})
	if err != nil {
		return archiveRelease{}, err
	}
	for _, a := range list {
		pkg := a.Binary.Package
		sum := hashx.Of(hashx.SHA256, pkg.Checksum)
		if pkg.Link != "" && sum.Valid() {
			return archiveRelease{url: pkg.Link, name: pkg.Name, version: a.Version.Semver, size: pkg.Size, sum: sum}, nil
		}
	}
	return archiveRelease{}, errNoBuild
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

func (m *Manager) zulu(ctx context.Context, major int) (archiveRelease, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH]
	osName := map[string]string{"linux": "linux", "darwin": "macos", "windows": "windows"}[runtime.GOOS]
	if arch == "" || osName == "" {
		return archiveRelease{}, errNoBuild
	}
	q := url.Values{
		"java_version": {strconv.Itoa(major)}, "os": {osName}, "arch": {arch},
		"archive_type": {strings.TrimPrefix(archiveExt(), ".")}, "java_package_type": {"jre"},
		"javafx_bundled": {"false"}, "release_status": {"ga"}, "availability_types": {"CA"}, "latest": {"true"},
	}
	type pkg struct {
		UUID        string `json:"package_uuid"`
		Name        string `json:"name"`
		URL         string `json:"download_url"`
		JavaVersion []int  `json:"java_version"`
	}
	list, err := httpcache.GetJSON[[]pkg](ctx, m.cfg.Cache, "https://api.azul.com/metadata/v1/zulu/packages/?"+q.Encode(), httpcache.Policy{TTL: vendorAPITTL})
	if err != nil {
		return archiveRelease{}, err
	}
	// The API's lib_c_type and crac filters are unreliable, so musl and CRaC
	// builds are skipped by name.
	i := slices.IndexFunc(list, func(p pkg) bool {
		return uuidRe.MatchString(p.UUID) && !strings.Contains(p.Name, "musl") && !strings.Contains(p.Name, "crac")
	})
	if i < 0 {
		return archiveRelease{}, errNoBuild
	}
	chosen := list[i]
	type details struct {
		SHA256 string `json:"sha256_hash"`
		Size   int64  `json:"size"`
	}
	d, err := httpcache.GetJSON[details](ctx, m.cfg.Cache, "https://api.azul.com/metadata/v1/zulu/packages/"+chosen.UUID, httpcache.Policy{TTL: vendorAPITTL})
	if err != nil {
		return archiveRelease{}, err
	}
	sum := hashx.Of(hashx.SHA256, d.SHA256)
	if !sum.Valid() {
		return archiveRelease{}, errNoBuild
	}
	parts := make([]string, len(chosen.JavaVersion))
	for i, n := range chosen.JavaVersion {
		parts[i] = strconv.Itoa(n)
	}
	// Azul's size is approximate, so it only drives progress; the sha256 verifies.
	return archiveRelease{url: chosen.URL, name: chosen.Name, version: strings.Join(parts, "."), estimate: d.Size, sum: sum}, nil
}
