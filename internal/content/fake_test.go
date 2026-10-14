package content

import (
	"context"

	"quartz/internal/modplatform"
)

// fakePlatform is an in-memory mod platform.
type fakePlatform struct {
	src       modplatform.Source
	byHash    map[string]modplatform.Version   // sha1 → the version that file is
	projects  map[string]modplatform.Project   // project id → project
	latest    map[string]modplatform.Version   // project id → newest version
	versions  map[string][]modplatform.Version // project id → versions, newest first
	changelog map[string]string                // version id → changelog
	notReady  bool
}

func (f *fakePlatform) Source() modplatform.Source { return f.src }
func (f *fakePlatform) Ready() bool                { return !f.notReady }

func (f *fakePlatform) Identify(_ context.Context, files []modplatform.Hashes) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	for _, h := range files {
		if v, ok := f.byHash[h.SHA1]; ok {
			out[h.SHA1] = v
		}
	}
	return out, nil
}

func (f *fakePlatform) Projects(_ context.Context, ids []string) (map[string]modplatform.Project, error) {
	out := map[string]modplatform.Project{}
	for _, id := range ids {
		if p, ok := f.projects[id]; ok {
			p.ID, p.Source = id, f.src
			out[id] = p
		}
	}
	return out, nil
}

func (f *fakePlatform) Latest(_ context.Context, items []modplatform.Installed, _ string, _ modplatform.Query) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	for _, it := range items {
		if v, ok := f.latest[it.ProjectID]; ok {
			out[it.Key] = v
		}
	}
	return out, nil
}

func (f *fakePlatform) Versions(_ context.Context, project string, _ modplatform.Query) ([]modplatform.Version, error) {
	return f.versions[project], nil
}

func (f *fakePlatform) Changelogs(_ context.Context, vs []modplatform.Version) error {
	for i := range vs {
		vs[i].Changelog = f.changelog[vs[i].ID]
	}
	return nil
}

// fakeCatalog adds the browsing side, enough to read versions by id.
type fakeCatalog struct{ *fakePlatform }

func (f fakeCatalog) Search(context.Context, modplatform.SearchQuery) (modplatform.SearchPage, error) {
	return modplatform.SearchPage{}, nil
}
func (f fakeCatalog) Project(context.Context, string) (modplatform.Detail, error) {
	return modplatform.Detail{}, nil
}
func (f fakeCatalog) Slug(context.Context, modplatform.ProjectType, string) (string, error) {
	return "", nil
}
func (f fakeCatalog) Version(_ context.Context, _, id string) (modplatform.Version, error) {
	vs, _ := f.Files(context.Background(), []string{id})
	return vs[id], nil
}
func (f fakeCatalog) Files(_ context.Context, ids []string) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	for _, vs := range f.versions {
		for _, v := range vs {
			for _, id := range ids {
				if v.ID == id {
					out[id] = v
				}
			}
		}
	}
	return out, nil
}
func (f fakeCatalog) Categories(context.Context, modplatform.ProjectType) ([]modplatform.Category, error) {
	return nil, nil
}
