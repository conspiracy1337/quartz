package loader

import (
	"context"
	"encoding/xml"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"quartz/internal/core/httpcache"
	"quartz/internal/core/version"
	"quartz/internal/meta"
)

// forgeLike is Forge and NeoForge: both publish installers on a maven
// repository and differ only in where their version lists come from.
type forgeLike struct {
	kind Kind
	// repo is the maven repository base, ending in "/".
	repo string
	// artifact returns the maven group:artifact of build id.
	artifact func(id string) string
	// list returns every build for mc, unsorted.
	list func(ctx context.Context, mc string) ([]Version, error)
	installer
}

const (
	forgeRepo    = "https://maven.minecraftforge.net/"
	neoForgeRepo = "https://maven.neoforged.net/releases/"
)

func newForge(d Deps) *forgeLike {
	f := &forgeLike{kind: Forge, repo: forgeRepo, artifact: func(string) string { return "net.minecraftforge:forge" }}
	f.list = f.forgeList
	f.installer = newInstaller(d, Forge)
	return f
}

func newNeoForge(d Deps) *forgeLike {
	f := &forgeLike{kind: NeoForge, repo: neoForgeRepo, artifact: neoForgeArtifact}
	f.list = f.neoForgeList
	f.installer = newInstaller(d, NeoForge)
	return f
}

func (f *forgeLike) Kind() Kind { return f.kind }

func (f *forgeLike) Versions(ctx context.Context, mc string) ([]Version, error) {
	vs, err := f.list(ctx, mc)
	if err != nil {
		return nil, err
	}
	sortNewest(vs)
	return vs, nil
}

func (f *forgeLike) Profile(ctx context.Context, req Request, id string) (*Profile, error) {
	if !strings.HasPrefix(id, req.Minecraft+"-") && f.kind == Forge {
		return nil, fmt.Errorf("%w: Forge %s is not for Minecraft %s", ErrUnknownVersion, id, req.Minecraft)
	}
	coord := f.artifact(id) + ":" + id + ":installer"
	return f.prepare(ctx, req, id, coord, f.repo)
}

// mavenMetadata is a repository's maven-metadata.xml.
type mavenMetadata struct {
	Versions []string `xml:"versioning>versions>version"`
}

// forgeList reads every Forge build for mc from maven-metadata.xml. Forge has
// no beta flag; promotions_slim.json names the recommended (else latest)
// build, which is "Latest stable", and builds newer than it are offered as
// beta.
func (f *forgeLike) forgeList(ctx context.Context, mc string) ([]Version, error) {
	data, err := f.meta.Bytes(ctx, forgeRepo+"net/minecraftforge/forge/maven-metadata.xml", httpcache.Policy{TTL: listTTL, MaxSize: 8 << 20})
	if err != nil {
		return nil, err
	}
	var md mavenMetadata
	if err := xml.Unmarshal(data, &md); err != nil {
		return nil, fmt.Errorf("loader: Forge versions: %w", err)
	}
	promos, _ := meta.Fetch[struct {
		Promos map[string]string `json:"promos"`
	}](ctx, f.meta, "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json", httpcache.Policy{TTL: listTTL})
	stable := promos.Promos[mc+"-recommended"]
	if stable == "" {
		stable = promos.Promos[mc+"-latest"]
	}
	var out []Version
	for _, id := range md.Versions {
		name, ok := forgeName(id, mc)
		if !ok || !ValidVersionID(id) {
			continue
		}
		out = append(out, Version{ID: id, Name: name, Stable: stable == "" || version.Compare(name, stable) <= 0})
	}
	return out, nil
}

// forgeName is the build number Forge shows: "1.7.10-10.13.4.1614-1.7.10"
// is "10.13.4.1614" on Minecraft 1.7.10.
func forgeName(id, mc string) (string, bool) {
	name, ok := strings.CutPrefix(id, mc+"-")
	if !ok || name == "" {
		return "", false
	}
	name = strings.TrimSuffix(name, "-"+mc)
	return name, name != ""
}

// neoForgeArtifact: NeoForge's 1.20.1 builds still live under the forge
// artifact, as "1.20.1-47.1.106"; everything later is net.neoforged:neoforge.
func neoForgeArtifact(id string) string {
	if strings.HasPrefix(id, "1.20.1-") {
		return "net.neoforged:forge"
	}
	return "net.neoforged:neoforge"
}

func (f *forgeLike) neoForgeList(ctx context.Context, mc string) ([]Version, error) {
	artifact := "neoforge"
	if mc == "1.20.1" {
		artifact = "forge"
	}
	list, err := meta.Fetch[struct {
		Versions []string `json:"versions"`
	}](ctx, f.meta, "https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/"+artifact, httpcache.Policy{TTL: listTTL})
	if err != nil {
		return nil, err
	}
	var out []Version
	for _, id := range list.Versions {
		if !ValidVersionID(id) {
			continue
		}
		name, ok := id, neoForgeFor(id, mc)
		if mc == "1.20.1" {
			name, ok = strings.CutPrefix(id, "1.20.1-")
		}
		if ok {
			out = append(out, Version{ID: id, Name: name, Stable: !prerelease(id)})
		}
	}
	return out, nil
}

// neoForgeFor reports whether NeoForge build id targets Minecraft mc. NeoForge
// numbers its builds after the game: 1.21.1 is 21.1.x, 1.21 is 21.0.x, and the
// year-based 26.1 is 26.1.0.x. Parts compare as numbers, so 21.1 never
// matches 21.10.
func neoForgeFor(id, mc string) bool {
	want, ok := neoForgePrefix(mc)
	if !ok {
		return false
	}
	base, _, _ := strings.Cut(id, "-")
	parts := strings.Split(base, ".")
	if len(parts) <= len(want) {
		return false
	}
	for i, w := range want {
		if n, err := strconv.Atoi(parts[i]); err != nil || n != w {
			return false
		}
	}
	return true
}

func neoForgePrefix(mc string) ([]int, bool) {
	old := strings.HasPrefix(mc, "1.")
	mc = strings.TrimPrefix(mc, "1.")
	var nums []int
	for _, p := range strings.Split(mc, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false // snapshots have no NeoForge
		}
		nums = append(nums, n)
	}
	size := 3
	if old {
		size = 2
	}
	if len(nums) > size {
		return nil, false
	}
	for len(nums) < size {
		nums = append(nums, 0)
	}
	return nums, true
}

// sortNewest orders builds newest first by their displayed version.
func sortNewest(vs []Version) {
	slices.SortStableFunc(vs, func(a, b Version) int { return version.Compare(b.Name, a.Name) })
}
