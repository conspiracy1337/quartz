package api

import (
	"context"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/core/fsx"
	"quartz/internal/core/tasks"
	"quartz/internal/exporting"
	"quartz/internal/instance"
)

// ExportDeps wires the export facade.
type ExportDeps struct {
	Service   *exporting.Service
	Instances *instance.Store
	Tasks     *tasks.Engine
	Busy      func(id string) bool // a game is starting or running
	Hosting   func(id string) bool // nil in tests
}

// Exports writes instances out as packs or mod lists.
type Exports struct {
	ctx *Ctx
	d   ExportDeps
}

func NewExports(ctx *Ctx, d ExportDeps) *Exports { return &Exports{ctx: ctx, d: d} }

// ExportResult is a started export: a job for packs, the saved file's name
// for a mod list (written at once), or a shared list's code. Empty when the
// save dialog was cancelled.
type ExportResult struct {
	Job  string          `json:"job,omitempty"`
	File string          `json:"file,omitempty"`
	Code *exporting.Code `json:"code,omitempty"`
}

// Meta is what the export dialog fills in: what the last export was
// called, else the instance's name and version 1.0.0.
func (x *Exports) Meta(id string) (instance.ExportMeta, error) {
	if err := checkInstanceID(id); err != nil {
		return instance.ExportMeta{}, err
	}
	in, err := x.d.Instances.Get(id)
	if err != nil {
		return instance.ExportMeta{}, err
	}
	if in.Export != nil {
		return *in.Export, nil
	}
	return instance.ExportMeta{Name: in.Name, Version: "1.0.0"}, nil
}

// Preview lists what an export would hold and leave out.
func (x *Exports) Preview(id string, p exporting.Plan) (exporting.Preview, error) {
	if err := checkInstanceID(id); err != nil {
		return exporting.Preview{}, err
	}
	if !p.Valid() {
		return exporting.Preview{}, exporting.ErrPlan
	}
	return x.d.Service.Preview(x.ctx.Get(), id, p)
}

// Export asks where to save, then writes the export: packs as a download
// job (they can be large), mod lists immediately.
func (x *Exports) Export(id string, p exporting.Plan) (ExportResult, error) {
	if err := checkInstanceID(id); err != nil {
		return ExportResult{}, err
	}
	if !p.Valid() {
		return ExportResult{}, exporting.ErrPlan
	}
	if err := x.guard(id); err != nil {
		return ExportResult{}, err
	}
	if p.Format == exporting.List && p.List == exporting.ShareCode {
		code, err := x.d.Service.Share(x.ctx.Get(), id)
		return ExportResult{Code: &code}, err
	}
	in, err := x.d.Instances.Get(id)
	if err != nil {
		return ExportResult{}, err
	}
	name, ext := fsx.FileName(in.Name, "instance"), p.Ext()
	if p.Format == exporting.List {
		name += "-mods"
	}
	dest, err := saveFileDialog(x.ctx.Get(), wruntime.SaveDialogOptions{
		Title:           "Export " + in.Name,
		DefaultFilename: name + ext,
		Filters:         []wruntime.FileFilter{{DisplayName: strings.TrimPrefix(ext, ".") + " files (*" + ext + ")", Pattern: "*" + ext}},
	})
	if err != nil || dest == "" {
		return ExportResult{}, err
	}
	if !strings.EqualFold(filepath.Ext(dest), ext) {
		dest += ext
	}
	if p.Format == exporting.List {
		return ExportResult{File: filepath.Base(dest)}, x.d.Service.Write(x.ctx.Get(), id, p, dest)
	}
	p.Meta = p.Meta.Clean()
	if _, err := x.d.Instances.Update(id, func(i *instance.Instance) error { i.Export = &p.Meta; return nil }); err != nil {
		return ExportResult{}, err
	}
	spec := tasks.NewJob(tasks.Meta{Title: "Exporting " + in.Name, Subtitle: filepath.Base(dest), Art: in.TaskArt(), Kind: "export"}).
		Step("Packing files", tasks.Count, tasks.Func("write the export", func(ctx context.Context) error {
			if err := x.guard(id); err != nil {
				return err
			}
			return x.d.Service.Write(ctx, id, p, dest)
		}))
	job, err := x.d.Tasks.Submit(spec)
	return ExportResult{Job: job, File: filepath.Base(dest)}, err
}

// guard refuses while a game or server changes the files being read.
func (x *Exports) guard(id string) error {
	if x.d.Hosting != nil && x.d.Hosting(id) {
		return ErrHosting
	}
	if x.d.Busy != nil && x.d.Busy(id) {
		return ErrBusy
	}
	return nil
}
