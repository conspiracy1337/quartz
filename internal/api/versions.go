package api

import (
	"sync"

	"quartz/internal/core/sysinfo"
	"quartz/internal/install"
	"quartz/internal/loader"
	"quartz/internal/meta"
	"quartz/internal/mojang"
)

// Versions exposes Minecraft and mod loader version metadata.
type Versions struct {
	ctx       *Ctx
	meta      *meta.Service
	loaders   *loader.Registry
	installer *install.Installer
}

func NewVersions(ctx *Ctx, m *meta.Service, l *loader.Registry, in *install.Installer) *Versions {
	return &Versions{ctx: ctx, meta: m, loaders: l, installer: in}
}

// Estimate is how many bytes the first launch still downloads for a version
// and loader build (files other instances already share are not counted).
// loader "" (or "none") is vanilla; an empty build id means the newest stable.
func (v *Versions) Estimate(minecraft, kind, loaderVersion string) (int64, error) {
	ctx := v.ctx.Get()
	t := install.Target{Minecraft: minecraft}
	if kind != "" && kind != "none" {
		k, err := loader.ParseKind(kind)
		if err != nil {
			return 0, err
		}
		if loaderVersion == "" {
			b, err := v.loaders.Latest(ctx, k, minecraft)
			if err != nil {
				return 0, err
			}
			loaderVersion = b.ID
		}
		t.Loader, t.LoaderVersion = k, loaderVersion
	}
	res, err := v.installer.Resolve(ctx, t, "", mojang.CurrentEnv(sysinfo.OSVersion()))
	if err != nil {
		return 0, err
	}
	return v.installer.Estimate(res)
}

// Minecraft returns the supported versions grouped for the two-step picker.
func (v *Versions) Minecraft() ([]mojang.Family, error) {
	return v.meta.Families(v.ctx.Get())
}

// LoaderVersion is one build in the loader picker.
type LoaderVersion struct {
	ID       string `json:"id"`       // stored build id
	Name     string `json:"name"`     // shown, e.g. "47.3.0"
	Released int64  `json:"released"` // unix ms; 0 when the upstream doesn't say
}

// LoaderOption is one loader in the picker: its stable builds newest first
// (the first is "Latest stable"), then its betas.
type LoaderOption struct {
	Kind   string          `json:"kind"` // none | fabric | quilt | neoforge | forge
	Name   string          `json:"name"`
	Note   string          `json:"note"`
	Stable []LoaderVersion `json:"stable"`
	Beta   []LoaderVersion `json:"beta"`
}

// loaderNotes are the picker's one-line descriptions.
var loaderNotes = map[loader.Kind]string{
	loader.Fabric: "Light, updates fast", loader.Quilt: "Runs most Fabric mods",
	loader.NeoForge: "Modern Forge", loader.Forge: "Classic big mods",
}

// Loaders lists every loader's builds for one Minecraft version, fetched in
// parallel. A loader whose site is unreachable shows no builds rather than
// failing the whole picker.
func (v *Versions) Loaders(minecraft string) []LoaderOption {
	ctx := v.ctx.Get()
	out := make([]LoaderOption, len(loader.Kinds)+1)
	out[0] = LoaderOption{Kind: "none", Name: "None", Note: "Vanilla, no mods", Stable: []LoaderVersion{}, Beta: []LoaderVersion{}}
	var wg sync.WaitGroup
	for i, k := range loader.Kinds {
		out[i+1] = LoaderOption{Kind: string(k), Name: k.Name(), Note: loaderNotes[k], Stable: []LoaderVersion{}, Beta: []LoaderVersion{}}
		wg.Go(func() {
			builds, err := v.loaders.Versions(ctx, k, minecraft)
			if err != nil {
				return
			}
			o := &out[i+1]
			for _, b := range builds {
				lv := LoaderVersion{ID: b.ID, Name: b.Name}
				if b.Stable {
					o.Stable = append(o.Stable, lv)
				} else {
					o.Beta = append(o.Beta, lv)
				}
			}
		})
	}
	wg.Wait()
	return out
}
