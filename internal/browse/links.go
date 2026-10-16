package browse

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"quartz/internal/core/docs"
	"quartz/internal/modplatform"
)

// Website folders of each project type, for links in descriptions.
var (
	modrinthPaths = map[string]modplatform.ProjectType{
		"mod": modplatform.TypeMod, "plugin": modplatform.TypeMod, "project": modplatform.TypeMod, "modpack": modplatform.TypeModpack,
		"resourcepack": modplatform.TypeResourcePack, "shader": modplatform.TypeShader, "datapack": modplatform.TypeDataPack,
	}
	cursePaths = map[string]modplatform.ProjectType{
		"mc-mods": modplatform.TypeMod, "modpacks": modplatform.TypeModpack, "texture-packs": modplatform.TypeResourcePack,
		"shaders": modplatform.TypeShader, "data-packs": modplatform.TypeDataPack, "worlds": modplatform.TypeWorld,
	}
	linkSlugRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// ErrLink is returned for a project link the launcher can't open.
var ErrLink = errors.New("browse: this link isn't a project the launcher can open")

// projectLink names the project a website address points at
// ("modrinth/mod/sodium"), or "" for any other address.
func projectLink(href string) string {
	u, err := url.Parse(href)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") {
	case "modrinth.com":
		if len(segs) >= 2 && linkSlugRe.MatchString(segs[1]) {
			if t, ok := modrinthPaths[segs[0]]; ok {
				return string(modplatform.Modrinth) + "/" + string(t) + "/" + segs[1]
			}
		}
	case "curseforge.com":
		if len(segs) >= 3 && segs[0] == "minecraft" && linkSlugRe.MatchString(segs[2]) {
			if t, ok := cursePaths[segs[1]]; ok {
				return string(modplatform.CurseForge) + "/" + string(t) + "/" + segs[2]
			}
		}
	}
	return ""
}

// linkProjects marks description links that point at a project, so the
// page can open them in the launcher (Prism and XMCL do).
func linkProjects(blocks []docs.Block) {
	mark := func(spans []docs.Span) {
		for i := range spans {
			if spans[i].Href != "" {
				spans[i].Project = projectLink(spans[i].Href)
			}
		}
	}
	for i := range blocks {
		mark(blocks[i].Spans)
		for _, it := range blocks[i].Items {
			mark(it)
		}
	}
}

// Resolve turns a link projectLink named into a project reference.
func (s *Service) Resolve(ctx context.Context, link string) (string, error) {
	parts := strings.Split(link, "/")
	if len(parts) != 3 || !linkSlugRe.MatchString(parts[2]) || !modplatform.ProjectType(parts[1]).Valid() {
		return "", ErrLink
	}
	src := modplatform.Source(parts[0])
	p := s.platform(src)
	if p == nil {
		return "", ErrLink
	}
	id, err := cached(s.twins, "slug:"+link, 30*time.Minute, func() (string, error) {
		return p.Slug(ctx, modplatform.ProjectType(parts[1]), parts[2])
	})
	if err != nil || !modplatform.ValidID(id) {
		return "", ErrLink
	}
	return modplatform.RefOf(src, id).String(), nil
}
