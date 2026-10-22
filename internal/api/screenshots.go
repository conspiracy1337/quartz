package api

import (
	"context"
	"fmt"

	"quartz/internal/core/clipimg"
	"quartz/internal/core/events"
	"quartz/internal/core/procx"
	"quartz/internal/core/tasks"
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
	Tasks     *tasks.Engine
	// Quality is the JPEG quality the compression setting asks for (0: off).
	Quality func() int
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
	return clipimg.CopyImage(p)
}

// ShowInFolder selects a screenshot in the file manager.
func (a *Screenshots) ShowInFolder(id, name string) error {
	p, err := a.path(id, name)
	if err != nil {
		return err
	}
	return procx.Reveal(p)
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
	if err := checkInstanceID(id); err != nil {
		return err
	}
	// Decoded here: a big screenshot is over what an imported picture may be.
	img, err := a.d.Service.Image(id, name)
	if err != nil {
		return err
	}
	hash, err := lib.AddImage(img)
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

// Uncompressed is how many screenshots are still PNGs, and their bytes.
type Uncompressed struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

// PNGs counts the screenshots that Compress existing would re-save.
func (a *Screenshots) PNGs() Uncompressed {
	var u Uncompressed
	for _, p := range a.d.Service.PNGs() {
		u.Count, u.Bytes = u.Count+1, u.Bytes+p.Bytes
	}
	return u
}

// CompressAll re-saves every PNG screenshot as JPEG at the setting's
// quality (High when it is off), as a job; it returns the job's id.
func (a *Screenshots) CompressAll() (string, error) {
	q := a.d.Quality()
	if q == 0 {
		q = screenshots.Quality("high")
	}
	pngs := a.d.Service.PNGs()
	if len(pngs) == 0 {
		return "", nil
	}
	works := make([]tasks.Work, 0, len(pngs))
	for _, p := range pngs {
		works = append(works, tasks.Func(p.Name, func(ctx context.Context) error {
			_, err := a.d.Service.Compress(ctx, p.Instance, []string{p.Name}, q)
			return err
		}))
	}
	return a.d.Tasks.Submit(tasks.NewJob(tasks.Meta{Title: "Compressing screenshots", Subtitle: fmt.Sprintf("%d pictures", len(pngs)), Kind: "compress"}).
		Step("Saving as JPEG", tasks.Count, works...))
}
