package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/core/events"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/meta"
	"quartz/internal/sandbox"
	"quartz/internal/sandbox/rules"
	"quartz/internal/settings"
	"quartz/internal/syncing"
)

var errBadSandbox = errors.New("api: invalid sandbox choice")

// SandboxDeps wires the Sandbox facade.
type SandboxDeps struct {
	Service  *sandbox.Service
	Store    *instance.Store
	Syncing  *syncing.Service
	Java     *java.Manager
	Meta     *meta.Service
	Settings *settings.Store
	Bus      *events.Bus
	Root     string // the data folder, for short paths
}

// Sandbox exposes an instance's Sandbox page.
type Sandbox struct {
	ctx *Ctx
	d   SandboxDeps
}

func NewSandbox(ctx *Ctx, d SandboxDeps) *Sandbox { return &Sandbox{ctx: ctx, d: d} }

// SandboxRow is one row of "What the game can reach": a rule, or one of the
// player's own folders.
type SandboxRow struct {
	ID      string   `json:"id"`
	Folder  bool     `json:"folder"`
	Path    string   `json:"path"` // "" keeps the page's own description
	Level   string   `json:"level"`
	Choices []string `json:"choices"`
}

// SandboxView is the page's state.
type SandboxView struct {
	On     bool         `json:"on"`
	Preset string       `json:"preset"`
	Rows   []SandboxRow `json:"rows"`
}

// Status says whether games can be sandboxed on this system, and with what.
func (a *Sandbox) Status() sandbox.Status { return a.d.Service.Status() }

// View is the instance's sandbox choices with the paths they cover.
func (a *Sandbox) View(id string) (SandboxView, error) {
	if err := checkInstanceID(id); err != nil {
		return SandboxView{}, err
	}
	in, err := a.d.Store.Get(id)
	if err != nil {
		return SandboxView{}, err
	}
	return a.view(in), nil
}

func (a *Sandbox) view(in instance.Instance) SandboxView {
	s, goos := in.Sandbox, runtime.GOOS
	v := SandboxView{On: s.On, Preset: string(s.Preset)}
	paths := map[rules.ID]string{
		rules.Instance:  a.short(a.d.Store.Dir(in.ID)) + "/",
		rules.Libraries: a.short(filepath.Join(a.d.Root, "store", "libraries")) + "/, " + a.short(filepath.Join(a.d.Root, "store", "assets")) + "/",
		rules.Java:      a.javaPath(in),
		rules.Synced:    a.syncedPaths(in.ID),
		rules.Graphics:  sandbox.Desktop(),
	}
	if in.Location != "" {
		paths[rules.Instance] = a.short(in.Location) + "/"
	}
	for _, r := range rules.Table {
		v.Rows = append(v.Rows, SandboxRow{ID: string(r.ID), Path: paths[r.ID], Level: string(s.Level(r.ID, goos)), Choices: levels(r.Choices(goos))})
	}
	for _, f := range s.Folders {
		v.Rows = append(v.Rows, SandboxRow{ID: f.ID, Folder: true, Path: a.short(f.Path), Level: string(f.Level),
			Choices: append(levels(rules.FolderChoices), string(rules.Remove))})
	}
	return v
}

func levels(ls []rules.Level) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = string(l)
	}
	return out
}

// short shows a path inside the data folder relative to it, one in the
// home folder as ~/…, and anything else in full.
func (a *Sandbox) short(p string) string {
	if rel, err := filepath.Rel(a.d.Root, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return p
}

// javaPath is the Java runtime the instance will run with, when it is
// picked or installed already (no download is planned for the page).
func (a *Sandbox) javaPath(in instance.Instance) string {
	const later = "Picked when you play"
	if eff := in.Resolve(a.d.Settings.Get()); eff.JavaPath != "" {
		return a.short(sandbox.JavaRoot(eff.JavaPath)) + "/"
	}
	if a.d.Meta == nil || a.d.Java == nil {
		return later
	}
	e, err := a.d.Meta.Entry(a.ctx.Get(), in.Minecraft)
	if err != nil {
		return later
	}
	for _, rt := range a.d.Java.Runtimes(a.ctx.Get()) {
		if rt.Major == e.EstimatedJava() {
			return a.short(sandbox.JavaRoot(rt.Path)) + "/"
		}
	}
	return later
}

// syncedPaths lists the shared copies the instance joins.
func (a *Sandbox) syncedPaths(id string) string {
	if a.d.Syncing == nil {
		return "None shared"
	}
	var out []string
	for _, t := range a.d.Syncing.Joined(id) {
		out = append(out, a.short(a.d.Syncing.SharedPath(t)))
	}
	if len(out) == 0 {
		return "None shared"
	}
	return strings.Join(out, ", ")
}

func (a *Sandbox) edit(id string, fn func(*rules.Settings) error) (SandboxView, error) {
	if err := checkInstanceID(id); err != nil {
		return SandboxView{}, err
	}
	in, err := a.d.Store.EditSandbox(id, fn)
	if err != nil {
		return SandboxView{}, err
	}
	a.d.Bus.Emit(events.InstancesChanged, nil)
	return a.view(in), nil
}

// SetOn turns the sandbox on or off; it applies from the next launch.
func (a *Sandbox) SetOn(id string, on bool) (SandboxView, error) {
	return a.edit(id, func(s *rules.Settings) error { s.On = on; return nil })
}

// SetPreset picks Standard, Strict or Custom.
func (a *Sandbox) SetPreset(id, preset string) (SandboxView, error) {
	p := rules.Preset(preset)
	if !slices.Contains([]rules.Preset{rules.Standard, rules.Strict, rules.Custom}, p) {
		return SandboxView{}, errBadSandbox
	}
	return a.edit(id, func(s *rules.Settings) error { *s = s.Apply(p, runtime.GOOS); return nil })
}

// SetLevel changes one rule, which makes the choices Custom.
func (a *Sandbox) SetLevel(id, rule, level string) (SandboxView, error) {
	return a.edit(id, func(s *rules.Settings) error {
		next, ok := s.SetLevel(rules.ID(rule), rules.Level(level), runtime.GOOS)
		if !ok {
			return errBadSandbox
		}
		*s = next
		return nil
	})
}

// AddFolder asks for a folder (a native dialog; the path never comes from
// the page) and allows it read-only.
func (a *Sandbox) AddFolder(id string) (SandboxView, error) {
	if err := checkInstanceID(id); err != nil {
		return SandboxView{}, err
	}
	dir, err := wruntime.OpenDirectoryDialog(a.ctx.Get(), wruntime.OpenDialogOptions{Title: "Choose a folder the game may reach"})
	if err != nil || dir == "" {
		if err == nil {
			return a.View(id)
		}
		return SandboxView{}, err
	}
	return a.addFolder(id, dir)
}

func (a *Sandbox) addFolder(id, dir string) (SandboxView, error) {
	dir = filepath.Clean(dir)
	if err := a.d.Service.CheckFolder(dir); err != nil {
		return SandboxView{}, err
	}
	return a.edit(id, func(s *rules.Settings) error {
		if slices.ContainsFunc(s.Folders, func(f rules.Folder) bool { return f.Path == dir }) {
			return nil
		}
		if len(s.Folders) >= rules.MaxFolders {
			return errBadSandbox
		}
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		s.Folders = append(s.Folders, rules.Folder{ID: hex.EncodeToString(b), Path: dir, Level: rules.RO})
		return nil
	})
}

// SetFolder changes one of the player's folders: "rw", "ro" or "remove".
func (a *Sandbox) SetFolder(id, folder, level string) (SandboxView, error) {
	if !rules.ValidFolderID(folder) {
		return SandboxView{}, errBadSandbox
	}
	l := rules.Level(level)
	if l != rules.Remove && !slices.Contains(rules.FolderChoices, l) {
		return SandboxView{}, errBadSandbox
	}
	return a.edit(id, func(s *rules.Settings) error {
		i := slices.IndexFunc(s.Folders, func(f rules.Folder) bool { return f.ID == folder })
		if i < 0 {
			return errBadSandbox
		}
		if l == rules.Remove {
			s.Folders = slices.Delete(slices.Clone(s.Folders), i, i+1)
		} else {
			s.Folders = slices.Clone(s.Folders)
			s.Folders[i].Level = l
		}
		return nil
	})
}
