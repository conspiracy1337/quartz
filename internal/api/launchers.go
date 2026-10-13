package api

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"quartz/internal/core/tasks"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/launchers"
	"quartz/internal/meta"
	"quartz/internal/modpack"
)

// LaunchersDeps are what bringing instances over from other launchers uses.
type LaunchersDeps struct {
	Instances *instance.Store
	Packs     *modpack.Service
	Meta      *meta.Service
	Java      *java.Manager
	Tasks     *tasks.Engine
	Opens     *Opens
	Log       *slog.Logger
	// Env says where to look (launchers.SystemEnv; a scratch folder in tests).
	Env launchers.Env
}

// Launchers is first-run setup's "Bring your instances" (SetupC).
type Launchers struct {
	ctx *Ctx
	d   LaunchersDeps

	mu    sync.Mutex
	found []launchers.Found // the last detection
}

// NewLaunchers returns the facade.
func NewLaunchers(ctx *Ctx, d LaunchersDeps) *Launchers { return &Launchers{ctx: ctx, d: d} }

// OtherLauncher is one row of the setup list.
type OtherLauncher struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Art       instance.Art `json:"art"`
	Summary   string       `json:"summary"`   // "6 instances", "6 instances, 2 already here"
	Instances int          `json:"instances"` // not brought over yet
}

// The setup rows' tiles: the design's colours, palette ones for the rest.
var launcherArt = map[string]instance.Art{
	"prism": {Background: "#b48cf2", Initial: "P"}, "modrinth": {Background: "#3ecf8e", Initial: "M"},
	"curseforge": {Background: "#f2a25c", Initial: "C"}, "multimc": {Background: "#e2a03f", Initial: "M"},
	"atlauncher": {Background: "#6b8afd", Initial: "A"},
}

// List finds the other launchers on this computer and their instances.
func (l *Launchers) List() []OtherLauncher {
	found := launchers.Detect(l.d.Env)
	l.mu.Lock()
	l.found = found
	l.mu.Unlock()
	out := []OtherLauncher{}
	for _, f := range found {
		fresh := l.fresh(f)
		here := len(f.Instances) - len(fresh)
		summary := plural(len(f.Instances), "instance")
		if here > 0 {
			summary += fmt.Sprintf(", %d already here", here)
		}
		out = append(out, OtherLauncher{ID: f.ID, Name: f.Name, Art: launcherArt[f.ID], Summary: summary, Instances: len(fresh)})
	}
	return out
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// fresh are f's instances not brought over before.
func (l *Launchers) fresh(f launchers.Found) []launchers.Source {
	have := map[string]bool{}
	for _, in := range l.d.Instances.List() {
		if in.ImportedFrom != nil {
			have[in.ImportedFrom.Path] = true
		}
	}
	return slices.DeleteFunc(slices.Clone(f.Instances), func(s launchers.Source) bool { return have[s.GameDir] })
}

// picked are the fresh instances of the chosen launchers.
func (l *Launchers) picked(ids []string) []struct {
	f launchers.Found
	s launchers.Source
} {
	l.mu.Lock()
	found := l.found
	l.mu.Unlock()
	var out []struct {
		f launchers.Found
		s launchers.Source
	}
	for _, f := range found {
		if !slices.Contains(ids, f.ID) {
			continue
		}
		for _, s := range l.fresh(f) {
			out = append(out, struct {
				f launchers.Found
				s launchers.Source
			}{f, s})
		}
	}
	return out
}

// JavaNeed is the setup card: the Java versions the picked instances need
// that aren't installed, and their download size.
type JavaNeed struct {
	Majors []int `json:"majors"`
	Bytes  int64 `json:"bytes"`
}

// Java says which Java runtimes importing ids would download.
func (l *Launchers) Java(ids []string) JavaNeed {
	need := JavaNeed{Majors: []int{}}
	for _, p := range l.missingJava(l.ctx.Get(), ids) {
		need.Majors = append(need.Majors, p.Major)
		need.Bytes += p.Bytes()
	}
	return need
}

func (l *Launchers) missingJava(ctx context.Context, ids []string) []*java.Plan {
	majors := map[int]string{}
	for _, it := range l.picked(ids) {
		v, err := l.d.Meta.Version(ctx, it.s.Minecraft)
		if err != nil {
			continue
		}
		majors[v.JavaMajor()] = v.JavaComponent()
	}
	var out []*java.Plan
	for major, component := range majors {
		if _, plan, err := l.d.Java.Resolve(ctx, java.Request{Major: major, Component: component}); err == nil && plan != nil {
			out = append(out, plan)
		}
	}
	slices.SortFunc(out, func(a, b *java.Plan) int { return a.Major - b.Major })
	return out
}

// importTimeout bounds the whole background import.
const importTimeout = 6 * time.Hour

// Import brings the fresh instances of the launchers ids over, copied or
// (inPlace) played in the other launcher's folders. It returns at once:
// missing Java runtimes download first, as their own jobs, then every
// instance installs as a job on the Downloads page. Problems that stop an
// instance before its job starts show as toasts.
func (l *Launchers) Import(ids []string, inPlace bool) error {
	for _, id := range ids {
		if _, ok := launcherArt[id]; !ok {
			return ErrBadSource
		}
	}
	items := l.picked(ids)
	go func() { //nolint:gosec // G118: the import outlives the request
		ctx, cancel := context.WithTimeout(context.Background(), importTimeout)
		defer cancel()
		l.downloadJava(ctx, ids)
		for _, it := range items {
			p := it.s.Pack(it.f, inPlace)
			tok := l.d.Packs.Stage(p)
			if _, err := l.d.Packs.Install(ctx, tok, p.Name, instance.ArtFor(p.Name)); err != nil {
				l.d.Log.Warn("importing an instance failed", "launcher", it.f.ID, "instance", p.Name, "err", err)
				l.d.Opens.Fail("Couldn't import "+p.Name, err)
			}
		}
	}()
	return nil
}

// downloadJava installs the missing runtimes first, so the instance jobs
// don't each download the same one.
func (l *Launchers) downloadJava(ctx context.Context, ids []string) {
	var jobs []string
	for _, plan := range l.missingJava(ctx, ids) {
		spec := tasks.NewJob(tasks.Meta{Title: fmt.Sprintf("Java %d runtime", plan.Major), Subtitle: java.VendorName(plan.Vendor),
			Art: tasks.Art{Background: "#5a5a63", Initial: "J"}, Kind: "java"})
		if id, err := l.d.Tasks.Submit(plan.AddTo(spec)); err == nil {
			jobs = append(jobs, id)
		}
	}
	for _, id := range jobs {
		_ = l.d.Tasks.Wait(ctx, id)
	}
}
