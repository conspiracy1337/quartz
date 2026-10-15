package browse

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"quartz/internal/modplatform"
)

// Why a platform is shown and installed from by default.
const (
	ReasonOnly  = "only"  // the project is on one platform
	ReasonSame  = "same"  // both have the same latest release
	ReasonNewer = "newer" // the preferred platform has a newer release
)

// releaseGrace is how much later a release must be published on one
// platform to count as newer: authors often upload separate builds of the
// same release to each platform hours apart.
const releaseGrace = 24 * time.Hour

// Prefer is the platform to install a project from by default. With an
// instance, only versions for its Minecraft version and loader count.
func (s *Service) Prefer(ctx context.Context, ref modplatform.Ref, instance string) (modplatform.Source, error) {
	tgt, err := s.target(instance)
	if err != nil {
		return "", err
	}
	src, _ := prefer(ref.Sources(), s.releases(ctx, ref), s.d.Channel(), tgt)
	return src, nil
}

// releases loads every platform's version list of a project at once. A
// platform whose list could not be read is missing.
func (s *Service) releases(ctx context.Context, ref modplatform.Ref) map[modplatform.Source][]modplatform.Version {
	var mu sync.Mutex
	out := map[modplatform.Source][]modplatform.Version{}
	var wg sync.WaitGroup
	for _, src := range ref.Sources() {
		wg.Go(func() {
			vs, err := s.allVersions(ctx, src, ref.ID(src))
			if err != nil {
				return
			}
			mu.Lock()
			out[src] = vs
			mu.Unlock()
		})
	}
	wg.Wait()
	return out
}

// prefer picks the platform to show and install from by default: Modrinth,
// which pays creators more, unless CurseForge has a newer release. A
// CurseForge release is newer only when Modrinth lacks its file and it came
// out more than releaseGrace after Modrinth's newest. The reason is "" when
// a version list could not be read.
func prefer(have []modplatform.Source, vers map[modplatform.Source][]modplatform.Version, ch modplatform.Channel, tgt *Target) (modplatform.Source, string) {
	if len(have) == 1 {
		return have[0], ReasonOnly
	}
	mrAll, mrOK := vers[modplatform.Modrinth]
	cfAll, cfOK := vers[modplatform.CurseForge]
	switch {
	case !cfOK:
		return modplatform.Modrinth, ""
	case !mrOK:
		return modplatform.CurseForge, ""
	}
	mr, mrAny := newest(mrAll, ch, tgt)
	cf, cfAny := newest(cfAll, ch, tgt)
	switch {
	case !cfAny && !mrAny:
		return modplatform.Modrinth, ReasonSame
	case !cfAny:
		return modplatform.Modrinth, ReasonNewer
	case !mrAny:
		return modplatform.CurseForge, ReasonNewer
	case mr.Published.After(cf.Published.Add(releaseGrace)):
		return modplatform.Modrinth, ReasonNewer
	case cf.Published.After(mr.Published.Add(releaseGrace)) && !hasFile(mrAll, cf.File.SHA1):
		return modplatform.CurseForge, ReasonNewer
	}
	return modplatform.Modrinth, ReasonSame
}

// newest is the most recently published version the channel offers and,
// with a target, that runs on it.
func newest(vs []modplatform.Version, ch modplatform.Channel, tgt *Target) (modplatform.Version, bool) {
	var out modplatform.Version
	found := false
	for _, v := range vs {
		t := v.Type
		if t == "" {
			t = "release"
		}
		if !ch.Allows(t) || (tgt != nil && !fits(v, tgt)) {
			continue
		}
		if !found || v.Published.After(out.Published) {
			out, found = v, true
		}
	}
	return out, found
}

// hasFile reports whether any version's file has the SHA-1.
func hasFile(vs []modplatform.Version, sha1 string) bool {
	return sha1 != "" && slices.ContainsFunc(vs, func(v modplatform.Version) bool { return strings.EqualFold(v.File.SHA1, sha1) })
}
