// Package loader installs mod loaders straight from their upstreams, with no
// launcher meta server in between: Fabric and Quilt from their meta APIs,
// Forge and NeoForge by reading and running their own installers (Pandora's
// approach, with installer steps skipped when their output already matches).
//
// A loader is described by a Profile: a version JSON that inherits from the
// Minecraft version, plus the extra files and post-processing the install
// package needs. The install package merges it over vanilla and plans one job.
package loader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"

	"quartz/internal/core/download"
	"quartz/internal/meta"
	"quartz/internal/mojang"
)

// Kind names a loader as instance.json stores it.
type Kind string

// The supported loaders.
const (
	Fabric   Kind = "fabric"
	Quilt    Kind = "quilt"
	Forge    Kind = "forge"
	NeoForge Kind = "neoforge"
)

// Kinds lists the loaders in the order the UI shows them.
var Kinds = []Kind{Fabric, Quilt, NeoForge, Forge}

// Errors callers match on.
var (
	ErrUnknownKind    = errors.New("loader: unknown mod loader")
	ErrUnknownVersion = errors.New("loader: unknown loader version")
	ErrNoBuild        = errors.New("loader: no build for this Minecraft version")
)

// ParseKind validates a stored or requested loader name. An unknown name is an
// error, never a silent vanilla (Pandora's bug).
func ParseKind(s string) (Kind, error) {
	k := Kind(s)
	if slices.Contains(Kinds, k) {
		return k, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownKind, s)
}

// Name is the loader's display name.
func (k Kind) Name() string {
	switch k {
	case Fabric:
		return "Fabric"
	case Quilt:
		return "Quilt"
	case Forge:
		return "Forge"
	case NeoForge:
		return "NeoForge"
	}
	return string(k)
}

// Version is one installable loader build for one Minecraft version.
type Version struct {
	ID     string `json:"id"`   // what instance.json stores and the installer needs, e.g. "1.20.1-47.3.0"
	Name   string `json:"name"` // what the UI shows, e.g. "47.3.0"
	Stable bool   `json:"stable"`
}

// versionID is the shape of every loader version id from every upstream.
var versionID = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)

// ValidVersionID reports whether s is a well-formed loader version id.
func ValidVersionID(s string) bool { return versionID.MatchString(s) }

// Request is what a Profile needs to know about the game it extends.
type Request struct {
	Minecraft string
	Env       mojang.Env      // the platform, for rules in installer library lists
	Parent    *mojang.Version // the vanilla version JSON
	ClientJar string          // where the vanilla client jar is (or will be) downloaded
	// Server prepares a dedicated server instead of the client; ServerJar is
	// where the vanilla server jar is (or will be) downloaded.
	Server    bool
	ServerJar string
}

// side is the installer side a request prepares.
func (r Request) side() string {
	if r.Server {
		return "server"
	}
	return "client"
}

// gameJar is the vanilla jar the request builds on.
func (r Request) gameJar() string {
	if r.Server {
		return r.ServerJar
	}
	return r.ClientJar
}

// Profile is everything a loader adds to a launch.
type Profile struct {
	// Version inherits from the Minecraft version; install merges it over vanilla.
	Version *mojang.Version
	// Extra are files only the install needs (installer tools and data),
	// downloaded with the game's libraries.
	Extra []mojang.ResolvedLibrary
	// Produced are library paths (slash-separated, inside the libraries
	// folder) that Processing creates, so they are neither downloaded nor
	// required to exist before it runs.
	Produced map[string]bool
	// Processing runs after downloads and Java are in place; nil when nothing runs.
	Processing *Processing
	// NoClientJar keeps the vanilla jar off the classpath (processed Forge
	// builds put a patched copy there instead).
	NoClientJar bool
	// Server says how a dedicated server starts; set only for server requests.
	Server *ServerStart
}

// ServerStart is how a loader's dedicated server starts, beyond the JVM's
// memory and user options (which come first) and the game's own arguments
// (which the caller appends).
type ServerStart struct {
	// Argv starts the server, e.g. "-jar forge-1.12.2-14.23.5.2859.jar" or
	// Forge's argument file. Empty means "-cp <libraries> <main class>".
	Argv []string
	// Place maps files Argv refers to relative to the server folder
	// ("libraries/…", "forge-….jar") to the store files they are.
	Place map[string]string
	// GameJarProperty is the system property pointing the loader at the
	// vanilla server jar (Fabric and Quilt); "" for none.
	GameJarProperty string
}

// Provider is one loader's upstream.
type Provider interface {
	Kind() Kind
	// Versions lists the builds for mc, newest first, stable ones marked.
	Versions(ctx context.Context, mc string) ([]Version, error)
	// Profile prepares one build. It may download the loader's installer, but
	// a build prepared once is served from disk.
	Profile(ctx context.Context, req Request, id string) (*Profile, error)
}

// Dirs are the store folders loaders write to.
type Dirs struct {
	Libraries string
	Versions  string
}

// Deps are what the providers share.
type Deps struct {
	Meta       *meta.Service
	Downloader *download.Downloader
	Verified   *download.Verified
	Dirs       Dirs
	Log        *slog.Logger
}

// Registry holds every provider.
type Registry struct {
	providers map[Kind]Provider
}

// New returns the registry with every supported loader.
func New(d Deps) *Registry {
	return NewWith(newFabric(d), newQuilt(d), newForge(d), newNeoForge(d))
}

// NewWith builds a registry from explicit providers (tests use fakes).
func NewWith(ps ...Provider) *Registry {
	r := &Registry{providers: map[Kind]Provider{}}
	for _, p := range ps {
		r.providers[p.Kind()] = p
	}
	return r
}

// Get returns the provider for k.
func (r *Registry) Get(k Kind) (Provider, error) {
	p, ok := r.providers[k]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, k)
	}
	return p, nil
}

// Versions lists the builds of k for mc.
func (r *Registry) Versions(ctx context.Context, k Kind, mc string) ([]Version, error) {
	p, err := r.Get(k)
	if err != nil {
		return nil, err
	}
	return p.Versions(ctx, mc)
}

// Latest returns the newest stable build of k for mc (the newest of any kind
// when none is marked stable), or ErrNoBuild.
func (r *Registry) Latest(ctx context.Context, k Kind, mc string) (Version, error) {
	vs, err := r.Versions(ctx, k, mc)
	if err != nil {
		return Version{}, err
	}
	return Pick(k, mc, vs)
}

// Pick is Latest over an already fetched list.
func Pick(k Kind, mc string, vs []Version) (Version, error) {
	if len(vs) == 0 {
		return Version{}, fmt.Errorf("%w: %s has no build for %s yet", ErrNoBuild, k.Name(), mc)
	}
	for _, v := range vs {
		if v.Stable {
			return v, nil
		}
	}
	return vs[0], nil
}

// Find returns the build id of k for mc, checking it exists upstream.
func (r *Registry) Find(ctx context.Context, k Kind, mc, id string) (Version, error) {
	vs, err := r.Versions(ctx, k, mc)
	if err != nil {
		return Version{}, err
	}
	for _, v := range vs {
		if v.ID == id {
			return v, nil
		}
	}
	return Version{}, fmt.Errorf("%w: %s %s for Minecraft %s", ErrUnknownVersion, k.Name(), id, mc)
}

// Match finds the build a modpack names, which may be the build id or the
// shown version ("47.2.0" for the id "1.20.1-47.2.0").
func (r *Registry) Match(ctx context.Context, k Kind, mc, version string) (Version, error) {
	vs, err := r.Versions(ctx, k, mc)
	if err != nil {
		return Version{}, err
	}
	for _, v := range vs {
		if v.ID == version || v.Name == version || strings.HasSuffix(v.ID, "-"+version) {
			return v, nil
		}
	}
	return Version{}, fmt.Errorf("%w: %s %s for Minecraft %s", ErrUnknownVersion, k.Name(), version, mc)
}

// Profile prepares build id of k.
func (r *Registry) Profile(ctx context.Context, k Kind, req Request, id string) (*Profile, error) {
	if !ValidVersionID(id) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownVersion, id)
	}
	p, err := r.Get(k)
	if err != nil {
		return nil, err
	}
	return p.Profile(ctx, req, id)
}

// prerelease reports whether a loader version is marked beta, alpha or pre.
func prerelease(v string) bool {
	_, suffix, ok := strings.Cut(v, "-")
	if !ok {
		return false
	}
	s := strings.ToLower(suffix)
	return strings.Contains(s, "beta") || strings.Contains(s, "alpha") || strings.Contains(s, "pre") || strings.Contains(s, "rc")
}

// locks serialises preparation of one build directory across goroutines.
type locks struct{ m sync.Map }

func (l *locks) lock(key string) func() {
	v, _ := l.m.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Display is how a build is shown, without asking upstream: Forge's
// "1.20.1-47.3.0" is "47.3.0".
func Display(k Kind, mc, id string) string {
	switch k {
	case Forge:
		if name, ok := forgeName(id, mc); ok {
			return name
		}
	case NeoForge:
		if name, ok := strings.CutPrefix(id, mc+"-"); ok && name != "" {
			return name
		}
	}
	return id
}

// Label is "Fabric 0.16.10" or "Forge 47.3.0".
func Label(k Kind, mc, id string) string { return k.Name() + " " + Display(k, mc, id) }
