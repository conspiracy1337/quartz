package api

import (
	"fmt"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/core/tasks"
	"quartz/internal/java"
	"quartz/internal/meta"
	"quartz/internal/settings"
)

// Java exposes Settings → Java: the runtime list and its actions.
type Java struct {
	ctx      *Ctx
	java     *java.Manager
	meta     *meta.Service
	tasks    *tasks.Engine
	settings *settings.Store
}

func NewJava(ctx *Ctx, j *java.Manager, m *meta.Service, t *tasks.Engine, s *settings.Store) *Java {
	return &Java{ctx: ctx, java: j, meta: m, tasks: t, settings: s}
}

// JavaRuntime is one row of the runtime list.
type JavaRuntime struct {
	Major  int    `json:"major"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
	UsedBy int    `json:"usedBy"`
}

func (j *Java) list() []JavaRuntime {
	rts := j.java.Runtimes(j.ctx.Get())
	out := make([]JavaRuntime, len(rts))
	for i, r := range rts {
		out[i] = JavaRuntime{Major: r.Major, Name: r.Name(), Source: string(r.Source), Path: r.Path}
	}
	return out
}

// Runtimes lists managed and detected Java installations.
func (j *Java) Runtimes() []JavaRuntime { return j.list() }

// Scan searches for Java again.
func (j *Java) Scan() []JavaRuntime {
	j.java.Scan(j.ctx.Get())
	return j.list()
}

// AddFromFolder lets the user pick a Java installation folder.
func (j *Java) AddFromFolder() ([]JavaRuntime, error) {
	dir, err := wruntime.OpenDirectoryDialog(j.ctx.Get(), wruntime.OpenDialogOptions{Title: "Choose a Java installation folder"})
	if err != nil || dir == "" {
		return j.list(), err
	}
	if _, err := j.java.AddFolder(j.ctx.Get(), dir); err != nil {
		return j.list(), err
	}
	return j.list(), nil
}

// Download installs the Java version the newest Minecraft release needs, from
// the vendor chosen in the settings, as a download job. It returns the job id.
func (j *Java) Download() (string, error) {
	ctx := j.ctx.Get()
	m, err := j.meta.Manifest(ctx)
	if err != nil {
		return "", err
	}
	v, err := j.meta.Version(ctx, m.Latest.Release)
	if err != nil {
		return "", err
	}
	plan, err := j.java.Plan(ctx, v.JavaMajor(), v.JavaComponent())
	if err != nil {
		return "", err
	}
	spec := tasks.NewJob(tasks.Meta{
		Title: fmt.Sprintf("Java %d runtime", plan.Major), Subtitle: java.VendorName(plan.Vendor),
		Art: tasks.Art{Background: "#5a5a63", Initial: "J"}, Kind: "java",
	})
	return j.tasks.Submit(plan.AddTo(spec))
}
