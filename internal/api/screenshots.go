package api

import (
	"quartz/internal/core/clipimg"
	"quartz/internal/core/events"
	"quartz/internal/core/procx"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/screenshots"
)

// ScreenshotDeps are what the screenshots facade works with.
type ScreenshotDeps struct {
	Service   *screenshots.Service
	Instances *instance.Store
	Icons     *icons.Library
	Banners   *icons.Library
	Bus       *events.Bus
}

// Screenshots exposes an instance's screenshots.
type Screenshots struct{ d ScreenshotDeps }

func NewScreenshots(d ScreenshotDeps) *Screenshots { return &Screenshots{d: d} }

func (a *Screenshots) path(id, name string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	return a.d.Service.Path(id, name)
}

// List returns the screenshots, newest first.
func (a *Screenshots) List(id string) ([]screenshots.Shot, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return a.d.Service.List(id)
}

// Copy puts a screenshot on the clipboard.
func (a *Screenshots) Copy(id, name string) error {
	p, err := a.path(id, name)
	if err != nil {
		return err
	}
	return clipimg.CopyPNG(p)
}

// ShowInFolder selects a screenshot in the file manager.
func (a *Screenshots) ShowInFolder(id, name string) error {
	p, err := a.path(id, name)
	if err != nil {
		return err
	}
	return procx.Reveal(p)
}

// OpenFolder opens the screenshots folder.
func (a *Screenshots) OpenFolder(id string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	return procx.OpenFolder(a.d.Instances.Path(id, "screenshots"))
}

// UseAsPicture makes a screenshot the instance's picture (cropped square).
func (a *Screenshots) UseAsPicture(id, name string) error {
	return a.use(id, name, a.d.Icons, a.d.Instances.SetIcon)
}

// UseAsBanner makes a screenshot the instance's banner.
func (a *Screenshots) UseAsBanner(id, name string) error {
	return a.use(id, name, a.d.Banners, a.d.Instances.SetBanner)
}

func (a *Screenshots) use(id, name string, lib *icons.Library, set func(id, hash string) (instance.Instance, error)) error {
	p, err := a.path(id, name)
	if err != nil {
		return err
	}
	hash, err := lib.Import(p)
	if err != nil {
		return err
	}
	if _, err := set(id, hash); err != nil {
		return err
	}
	a.d.Bus.Emit(events.InstancesChanged, nil)
	return nil
}

// Trash moves screenshots aside; the token undoes it for 15 minutes.
func (a *Screenshots) Trash(id string, names []string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	if len(names) == 0 || len(names) > 1000 {
		return "", screenshots.ErrNoShot
	}
	return a.d.Service.Trash(id, names)
}

// Undo brings trashed screenshots back.
func (a *Screenshots) Undo(id, token string) error {
	if err := checkUndo(id, token); err != nil {
		return err
	}
	return a.d.Service.Undo(id, token)
}
