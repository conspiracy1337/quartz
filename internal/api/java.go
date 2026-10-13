package api

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/core/procx"
	"quartz/internal/core/tasks"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/meta"
	"quartz/internal/settings"
)

// JavaDeps wires the Java facade.
type JavaDeps struct {
	Java      *java.Manager
	Meta      *meta.Service
	Tasks     *tasks.Engine
	Settings  *settings.Store
	Instances *instance.Store      // counts what uses each runtime; nil in tests
	Busy      func(id string) bool // a game is starting or running
}

// Java exposes Settings → Java: the runtime list and its actions.
type Java struct {
	ctx *Ctx
	d   JavaDeps
}

func NewJava(ctx *Ctx, d JavaDeps) *Java { return &Java{ctx: ctx, d: d} }

// JavaRuntime is one row of the runtime list.
type JavaRuntime struct {
	ID          string `json:"id"`
	Major       int    `json:"major"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	UsedBy      int    `json:"usedBy"`
	Removable   bool   `json:"removable"`   // downloaded by Quartz
	Forgettable bool   `json:"forgettable"` // added from a folder
	Default     bool   `json:"default"`     // every instance uses it (automatic picking off)
}

var (
	javaIDPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)
	errJavaID     = errors.New("api: not a Java runtime")
	errJavaInUse  = errors.New("api: instances use this Java. Pick another one for them first")
	errJavaMajor  = errors.New("api: that Java version can't be downloaded")
)

func (j *Java) list() []JavaRuntime {
	ctx := j.ctx.Get()
	rts := j.d.Java.Runtimes(ctx)
	used := j.usage()
	def := j.d.Settings.Get().Java
	out := make([]JavaRuntime, len(rts))
	for i, r := range rts {
		out[i] = JavaRuntime{
			ID: java.ID(r.Path), Major: r.Major, Name: r.Name(), Source: string(r.Source), Path: r.Path, UsedBy: used[r.Path],
			Removable: r.Source == java.Managed, Forgettable: j.d.Java.Added(r.Path),
			Default: !def.AutoPick && def.DefaultPath == r.Path,
		}
	}
	return out
}

// usage counts the instances that would start with each runtime: their own
// choice, the default one, or what automatic picking takes for their version.
func (j *Java) usage() map[string]int {
	out := map[string]int{}
	if j.d.Instances == nil {
		return out
	}
	ctx, g := j.ctx.Get(), j.d.Settings.Get()
	for _, in := range j.d.Instances.List() {
		if eff := in.Resolve(g); eff.JavaPath != "" {
			out[eff.JavaPath]++
			continue
		}
		e, err := j.d.Meta.Entry(ctx, in.Minecraft)
		if err != nil {
			continue
		}
		if rt, ok := j.d.Java.Pick(ctx, e.EstimatedJava()); ok {
			out[rt.Path]++
		}
	}
	return out
}

// find resolves a runtime id from the UI.
func (j *Java) find(id string) (java.Runtime, error) {
	if !javaIDPattern.MatchString(id) {
		return java.Runtime{}, errJavaID
	}
	rt, ok := j.d.Java.Find(j.ctx.Get(), id)
	if !ok {
		return java.Runtime{}, errJavaID
	}
	return rt, nil
}

// Runtimes lists managed and detected Java installations.
func (j *Java) Runtimes() []JavaRuntime { return j.list() }

// Scan searches for Java again.
func (j *Java) Scan() []JavaRuntime {
	j.d.Java.Scan(j.ctx.Get())
	return j.list()
}

// AddFromFolder lets the user pick a Java installation folder.
func (j *Java) AddFromFolder() ([]JavaRuntime, error) {
	dir, err := openDirDialog(j.ctx.Get(), wruntime.OpenDialogOptions{Title: "Choose a Java installation folder"})
	if err != nil || dir == "" {
		return j.list(), err
	}
	if _, err := j.d.Java.AddFolder(j.ctx.Get(), dir); err != nil {
		return j.list(), err
	}
	return j.list(), nil
}

// ShowInFolder shows a runtime's java binary in the file manager.
func (j *Java) ShowInFolder(id string) error {
	rt, err := j.find(id)
	if err != nil {
		return err
	}
	return procx.Reveal(rt.Path)
}

// Remove deletes a runtime Quartz downloaded, once nothing uses it.
func (j *Java) Remove(id string) ([]JavaRuntime, error) {
	rt, err := j.find(id)
	if err != nil {
		return j.list(), err
	}
	if err := j.idle(rt); err != nil {
		return j.list(), err
	}
	if err := j.d.Java.Remove(rt); err != nil {
		return j.list(), err
	}
	return j.list(), nil
}

// Forget takes a runtime the user added off the list.
func (j *Java) Forget(id string) ([]JavaRuntime, error) {
	rt, err := j.find(id)
	if err != nil {
		return j.list(), err
	}
	if !j.d.Java.Added(rt.Path) {
		return j.list(), errJavaID
	}
	if err := j.idle(rt); err != nil {
		return j.list(), err
	}
	j.d.Java.Forget(rt.Path)
	return j.list(), nil
}

// idle refuses while a game runs or an instance would start with rt.
func (j *Java) idle(rt java.Runtime) error {
	if j.d.Instances != nil && j.d.Busy != nil {
		for _, in := range j.d.Instances.List() {
			if j.d.Busy(in.ID) {
				return ErrBusy
			}
		}
	}
	if j.usage()[rt.Path] > 0 {
		return errJavaInUse
	}
	return nil
}

// SetDefault makes a runtime the one every instance uses while automatic
// picking is off. The path is set here only, never by the settings page.
func (j *Java) SetDefault(id string) ([]JavaRuntime, error) {
	rt, err := j.find(id)
	if err != nil {
		return j.list(), err
	}
	s := j.d.Settings.Get()
	s.Java.DefaultPath = rt.Path
	j.d.Settings.Replace(s)
	return j.list(), nil
}

// Majors are the Java versions Download offers, newest first.
func (j *Java) Majors() []int { return java.Majors() }

// Download installs a Java version from the vendor chosen in the settings, as
// a download job. It returns the job id.
func (j *Java) Download(major int) (string, error) {
	if !slices.Contains(java.Majors(), major) {
		return "", errJavaMajor
	}
	plan, err := j.d.Java.Plan(j.ctx.Get(), major, "")
	if err != nil {
		return "", err
	}
	spec := tasks.NewJob(tasks.Meta{
		Title: fmt.Sprintf("Java %d runtime", plan.Major), Subtitle: java.VendorName(plan.Vendor),
		Art: tasks.Art{Background: "#5a5a63", Initial: "J"}, Kind: "java",
	})
	return j.d.Tasks.Submit(plan.AddTo(spec))
}
