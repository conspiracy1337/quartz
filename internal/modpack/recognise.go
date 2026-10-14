package modpack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/events"
	"quartz/internal/core/hashx"
	"quartz/internal/core/safe"
	"quartz/internal/instance"
	"quartz/internal/modplatform"
)

// Recognition limits: candidates looked at, pack versions read per
// candidate, the share of a version's mods the instance must have, and the
// fewest mods worth judging by.
const (
	maxCandidates = 3
	maxReads      = 5
	minOverlap    = 0.85
	minMods       = 3
	recogniseTime = 10 * time.Minute
)

// ErrNotRecognised is returned when no pack matches an instance's files.
var ErrNotRecognised = errors.New("modpack: no modpack matches this instance's mods")

// trailingVersion is version text at the end of an instance name ("Pack
// 6.2", "Pack - 1.20.1", "Pack [FORGE]"), left out of the search.
var trailingVersion = regexp.MustCompile(`(?i)(\s*[\[(][^\])]*[\])]|[\s_-]+v?\d+(\.\d+)*[a-z]?)+\s*$`)

// recogniseLater runs Recognise in the background, for an import that
// just finished.
func (s *Service) recogniseLater(id string) {
	safe.Go(s.d.Log, "recognise modpack", func() {
		ctx, cancel := context.WithTimeout(context.Background(), recogniseTime)
		defer cancel()
		if err := s.Recognise(ctx, id); err != nil {
			s.d.Log.Debug("modpack: not recognised", "instance", id, "err", err)
		}
	})
}

// Recognise looks for the modpack an imported instance was installed from:
// it searches both platforms for packs named like the instance, then reads
// versions of each for the instance's Minecraft version and loader and
// keeps the one whose mods the instance has most of (at least 85%). The
// instance is then linked to that pack and version, so it gets updates.
// Instances played in another launcher's folder are never linked: an
// update would change that launcher's files.
func (s *Service) Recognise(ctx context.Context, id string) error {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return err
	}
	if in.InPlace || (in.Pack != nil && (Origin{Source: in.Pack.Source, Project: in.Pack.Project}).Updatable()) {
		return ErrNotRecognised
	}
	have := s.modSums(id)
	if len(have) < minMods {
		return ErrNotRecognised
	}
	var best *Pack
	bestShare := 0.0
	for _, c := range s.candidates(ctx, in.Name) {
		p, share := s.bestVersion(ctx, c, in, have)
		if p == nil {
			continue
		}
		if share > bestShare {
			s.d.Reader.Discard(best)
			best, bestShare = p, share
		} else {
			s.d.Reader.Discard(p)
		}
	}
	if best == nil || bestShare < minOverlap {
		s.d.Reader.Discard(best)
		return ErrNotRecognised
	}
	defer s.d.Reader.Discard(best)
	if _, err := s.baseline(s.d.Instances.Dir(id), best); err != nil {
		return err
	}
	if err := s.link(id, Managed{Origin: best.Origin, Name: best.Name, VersionName: best.Version}); err != nil {
		return err
	}
	if s.u.Record != nil {
		s.u.Record(id, content.Activity{Kind: "updated", Title: fmt.Sprintf("Recognised as %s %s (%s)", best.Name, best.Version, platformName(best.Origin.Source))})
	}
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.InstancesChanged, nil)
	}
	return nil
}

// candidate is a pack on one platform that might be the instance's.
type candidate struct {
	plat    modplatform.Platform
	project string
}

// candidates searches both platforms for packs whose title matches the
// instance's name, taking them in turn from each platform's ranking.
func (s *Service) candidates(ctx context.Context, name string) []candidate {
	text := strings.TrimSpace(trailingVersion.ReplaceAllString(name, ""))
	want := modplatform.Normalize(text)
	if want == "" {
		return nil
	}
	var lists [][]candidate
	for _, plat := range []modplatform.Platform{s.d.Reader.d.Modrinth, s.d.Reader.d.CurseForge} {
		if plat == nil || !plat.Ready() {
			continue
		}
		page, err := plat.Search(ctx, modplatform.SearchQuery{Type: modplatform.TypeModpack, Text: text, Sort: modplatform.SortRelevance, Limit: 10})
		if err != nil {
			continue
		}
		var list []candidate
		for _, h := range page.Hits {
			if t := modplatform.Normalize(h.Title); t != "" && (strings.Contains(t, want) || strings.Contains(want, t)) {
				list = append(list, candidate{plat: plat, project: h.ID})
			}
		}
		lists = append(lists, list)
	}
	var out []candidate
	for i := 0; len(out) < maxCandidates; i++ {
		added := false
		for _, l := range lists {
			if i < len(l) && len(out) < maxCandidates {
				out = append(out, l[i])
				added = true
			}
		}
		if !added {
			break
		}
	}
	return out
}

// bestVersion reads a candidate's versions for the instance's line, newest
// first, while the share of mods the instance has keeps rising (an older
// install matches an older version better), and returns the best one.
func (s *Service) bestVersion(ctx context.Context, c candidate, in instance.Instance, have map[string]bool) (*Pack, float64) {
	vs, err := c.plat.Versions(ctx, c.project, modplatform.Query{})
	if err != nil {
		return nil, 0
	}
	var best *Pack
	bestShare, reads := 0.0, 0
	for _, v := range vs {
		if !onLine(v, in) {
			continue
		}
		if reads++; reads > maxReads {
			break
		}
		p, err := s.d.Reader.Project(ctx, c.plat.Source(), c.project, v.ID)
		if err != nil {
			continue
		}
		share := overlap(p, have)
		if share <= bestShare {
			s.d.Reader.Discard(p)
			if bestShare >= minOverlap {
				break // past the installed version
			}
			continue
		}
		s.d.Reader.Discard(best)
		best, bestShare = p, share
		if share == 1 {
			break
		}
	}
	return best, bestShare
}

// overlap is the share of a pack version's mods whose sha1 the instance has.
func overlap(p *Pack, have map[string]bool) float64 {
	total, found := 0, 0
	for _, f := range p.Files {
		if strings.HasPrefix(f.Path, "mods/") && isContent(f.Path) {
			total++
			if have[strings.ToLower(f.SHA1)] {
				found++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(found) / float64(total)
}

// modSums hashes the instance's mods, turned on or off.
func (s *Service) modSums(id string) map[string]bool {
	out := map[string]bool{}
	dir := s.d.Instances.Path(id, "mods")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || !isContent("mods/"+e.Name()) {
			continue
		}
		if sum, err := hashx.File(filepath.Join(dir, e.Name()), hashx.SHA1); err == nil {
			out[strings.ToLower(sum)] = true
		}
	}
	return out
}

func platformName(src string) string {
	return map[string]string{"modrinth": "Modrinth", "curseforge": "CurseForge", "ftb": "FTB"}[src]
}
