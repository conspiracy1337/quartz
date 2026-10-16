package api

import (
	"errors"
	"regexp"

	"quartz/internal/browse"
	"quartz/internal/modplatform"
	"quartz/internal/worlds"
)

// Browse exposes the merged search and the project page.
type Browse struct {
	ctx    *Ctx
	svc    *browse.Service
	worlds *worlds.Service
}

// NewBrowse returns the facade.
func NewBrowse(ctx *Ctx, svc *browse.Service, w *worlds.Service) *Browse {
	return &Browse{ctx: ctx, svc: svc, worlds: w}
}

// InstallWorld downloads a CurseForge world (a version, or the newest) into
// an instance as a job and returns its id.
func (b *Browse) InstallWorld(instanceID, ref, version string) (string, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return "", err
	}
	r, err := modplatform.ParseRef(ref)
	if err != nil {
		return "", err
	}
	if version != "" && !versionIDRe.MatchString(version) {
		return "", ErrBadSource
	}
	title, f, err := b.svc.World(b.ctx.Get(), r, version)
	if err != nil {
		return "", err
	}
	return b.worlds.Install(instanceID, title, f)
}

// Validation errors.
var (
	ErrBadSearch = errors.New("api: invalid search")
	ErrBadSource = errors.New("api: invalid source")
)

var (
	searchIDRe  = regexp.MustCompile(`^[0-9a-f]{16}$`)
	versionIDRe = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)
)

func checkSource(src string) (modplatform.Source, error) {
	switch modplatform.Source(src) {
	case modplatform.Modrinth, modplatform.CurseForge, modplatform.NoRisk:
		return modplatform.Source(src), nil
	}
	return "", ErrBadSource
}

func checkOptionalInstance(id string) error {
	if id == "" {
		return nil
	}
	return checkInstanceID(id)
}

// Search starts a search and returns its first page.
func (b *Browse) Search(q browse.Query) (browse.Results, error) {
	if !modplatform.ProjectType(q.Type).Valid() || len(q.Text) > 200 || len(q.Categories) > 12 {
		return browse.Results{}, ErrBadSearch
	}
	if err := checkOptionalInstance(q.Instance); err != nil {
		return browse.Results{}, err
	}
	return b.svc.Search(b.ctx.Get(), q)
}

// More returns the next page of a search.
func (b *Browse) More(searchID string) (browse.Results, error) {
	if !searchIDRe.MatchString(searchID) {
		return browse.Results{}, browse.ErrUnknownSearch
	}
	return b.svc.More(b.ctx.Get(), searchID)
}

// Installed lists the projects an instance has, one "mr:<id>" or "cf:<id>"
// ref per platform.
func (b *Browse) Installed(instanceID string) ([]string, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	return asListErr(b.svc.Installed(instanceID))
}

// Categories lists a project type's categories.
func (b *Browse) Categories(t string) []browse.CategoryGroup {
	return asList(browse.Categories(modplatform.ProjectType(t)))
}

// Project reads a project page; instance (optional) is the target.
func (b *Browse) Project(ref, instance string) (browse.Page, error) {
	r, err := modplatform.ParseRef(ref)
	if err != nil {
		return browse.Page{}, err
	}
	if err := checkOptionalInstance(instance); err != nil {
		return browse.Page{}, err
	}
	return b.svc.Project(b.ctx.Get(), r, instance)
}

// Prefer is the platform to install a project from by default; instance
// (optional) narrows it to the versions that run there.
func (b *Browse) Prefer(ref, instance string) (string, error) {
	r, err := modplatform.ParseRef(ref)
	if err != nil {
		return "", err
	}
	if err := checkOptionalInstance(instance); err != nil {
		return "", err
	}
	src, err := b.svc.Prefer(b.ctx.Get(), r, instance)
	return string(src), err
}

// Versions lists a project's versions on one platform.
func (b *Browse) Versions(ref, source, instance string, compatible bool) ([]browse.VersionRow, error) {
	r, err := modplatform.ParseRef(ref)
	if err != nil {
		return nil, err
	}
	src, err := checkSource(source)
	if err != nil {
		return nil, err
	}
	if err := checkOptionalInstance(instance); err != nil {
		return nil, err
	}
	return asListErr(b.svc.Versions(b.ctx.Get(), r, src, instance, compatible))
}

// Changelog reads versions' changes (the newest five when ids is empty).
func (b *Browse) Changelog(ref, source string, ids []string) ([]browse.Notes, error) {
	r, err := modplatform.ParseRef(ref)
	if err != nil {
		return nil, err
	}
	src, err := checkSource(source)
	if err != nil {
		return nil, err
	}
	if len(ids) > 5 {
		return nil, ErrBadSearch
	}
	for _, id := range ids {
		if !versionIDRe.MatchString(id) {
			return nil, ErrBadSearch
		}
	}
	return asListErr(b.svc.Changelog(b.ctx.Get(), r, src, ids))
}

// Resolve opens a project a description links to ("modrinth/mod/sodium"):
// its reference for the project page.
func (b *Browse) Resolve(link string) (string, error) {
	if len(link) > 128 {
		return "", browse.ErrLink
	}
	return b.svc.Resolve(b.ctx.Get(), link)
}
