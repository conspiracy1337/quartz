package loader

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"quartz/internal/core/httpcache"
	"quartz/internal/meta"
	"quartz/internal/mojang"
)

const (
	listTTL    = 10 * time.Minute
	profileTTL = 365 * 24 * time.Hour // a published build's profile never changes
)

// fabricLike is Fabric's meta API, which Quilt copies: one implementation
// configured twice.
type fabricLike struct {
	kind Kind
	base string // e.g. https://meta.fabricmc.net/v2
	// profileID is the id the profile JSON must carry for build ver on mc.
	profileID func(ver, mc string) string
	meta      *meta.Service
}

func newFabric(d Deps) *fabricLike {
	return &fabricLike{kind: Fabric, base: "https://meta.fabricmc.net/v2", meta: d.Meta,
		profileID: func(ver, mc string) string { return "fabric-loader-" + ver + "-" + mc }}
}

func newQuilt(d Deps) *fabricLike {
	return &fabricLike{kind: Quilt, base: "https://meta.quiltmc.org/v3", meta: d.Meta,
		profileID: func(ver, mc string) string { return "quilt-loader-" + ver + "-" + mc }}
}

func (f *fabricLike) Kind() Kind { return f.kind }

// fabricEntry is one element of /versions/loader/{mc}. Fabric's own "stable"
// flag marks only its single newest stable build, so stability is read from
// the version itself, as Quilt writes it ("0.17.0-beta.1").
type fabricEntry struct {
	Loader struct {
		Version string `json:"version"`
	} `json:"loader"`
}

func (f *fabricLike) Versions(ctx context.Context, mc string) ([]Version, error) {
	list, err := meta.Fetch[[]fabricEntry](ctx, f.meta, f.base+"/versions/loader/"+url.PathEscape(mc), httpcache.Policy{TTL: listTTL})
	if err != nil {
		return nil, err
	}
	out := make([]Version, 0, len(list))
	for _, e := range list {
		v := e.Loader.Version
		if !ValidVersionID(v) {
			continue
		}
		out = append(out, Version{ID: v, Name: v, Stable: !prerelease(v)})
	}
	sortNewest(out)
	return out, nil
}

func (f *fabricLike) Profile(ctx context.Context, req Request, id string) (*Profile, error) {
	u := f.base + "/versions/loader/" + url.PathEscape(req.Minecraft) + "/" + url.PathEscape(id) + "/profile/json"
	data, err := f.meta.Bytes(ctx, u, httpcache.Policy{TTL: profileTTL, MaxSize: 1 << 20})
	if err != nil {
		return nil, err
	}
	v, err := mojang.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("loader: %s profile: %w", f.kind.Name(), err)
	}
	if want := f.profileID(id, req.Minecraft); v.ID != want || v.InheritsFrom != req.Minecraft {
		return nil, fmt.Errorf("loader: %s profile is for %q on %q, expected %q on %q", f.kind.Name(), v.ID, v.InheritsFrom, want, req.Minecraft)
	}
	return &Profile{Version: v}, nil
}
