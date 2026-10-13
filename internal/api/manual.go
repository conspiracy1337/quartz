package api

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"quartz/internal/manual"
)

// ManualDeps are what the manual-downloads facade works with.
type ManualDeps struct {
	Service *manual.Service
	Drops   *Drops
	App     *App
}

// Manual is the dialog for files only the CurseForge website serves.
type Manual struct {
	d ManualDeps
}

// NewManual returns the facade.
func NewManual(d ManualDeps) *Manual { return &Manual{d: d} }

var (
	jobIDRe    = regexp.MustCompile(`^[0-9a-f]{12}$`)
	manualIDRe = regexp.MustCompile(`^[0-9]{1,5}$`)
	// ErrBadJob is returned for an invalid job id.
	ErrBadJob = errors.New("api: invalid job")
)

// View lists a job's needed files.
func (m *Manual) View(jobID string) (manual.View, error) {
	if !jobIDRe.MatchString(jobID) {
		return manual.View{}, ErrBadJob
	}
	return m.d.Service.View(jobID)
}

// UseModrinth downloads one file from Modrinth instead.
func (m *Manual) UseModrinth(jobID, itemID string) error {
	if !jobIDRe.MatchString(jobID) || !manualIDRe.MatchString(itemID) {
		return ErrBadJob
	}
	return m.d.Service.UseModrinth(jobID, itemID)
}

// Skip goes on without the files still missing.
func (m *Manual) Skip(jobID string) error {
	if !jobIDRe.MatchString(jobID) {
		return ErrBadJob
	}
	return m.d.Service.Skip(jobID)
}

// AddDropped checks files dropped onto the dialog; it returns how many matched.
func (m *Manual) AddDropped(jobID, dropID string) (int, error) {
	if !jobIDRe.MatchString(jobID) {
		return 0, ErrBadJob
	}
	paths, err := m.d.Drops.Take(dropID)
	if err != nil {
		return 0, err
	}
	return m.d.Service.Offer(jobID, paths)
}

// OpenPages opens the CurseForge page of every file still missing.
func (m *Manual) OpenPages(jobID string) error {
	if !jobIDRe.MatchString(jobID) {
		return ErrBadJob
	}
	pages, err := m.d.Service.Pages(jobID)
	if err != nil {
		return err
	}
	for _, p := range pages {
		if u, err := url.Parse(p); err == nil && strings.HasSuffix(strings.ToLower(u.Hostname()), "curseforge.com") {
			if err := m.d.App.OpenURL(p); err != nil {
				return err
			}
		}
	}
	return nil
}
