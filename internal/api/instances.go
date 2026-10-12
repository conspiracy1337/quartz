package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/core/events"
	"quartz/internal/core/procx"
	"quartz/internal/core/sysinfo"
	"quartz/internal/core/tasks"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/loader"
	"quartz/internal/meta"
	"quartz/internal/settings"
)

// InstanceDeps are what the instances facade works with.
type InstanceDeps struct {
	Store    *instance.Store
	Meta     *meta.Service
	Loaders  *loader.Registry
	Icons    *icons.Library
	Settings *settings.Store
	Tasks    *tasks.Engine
	Bus      *events.Bus
	Running  func(id string) bool // the game is running
	Busy     func(id string) bool // starting or running: files must stay put
}

// Instances exposes the instance list and everything done to one instance.
type Instances struct {
	ctx *Ctx
	d   InstanceDeps
}

func NewInstances(ctx *Ctx, d InstanceDeps) *Instances { return &Instances{ctx: ctx, d: d} }

// LoaderRef is an instance's loader as the UI shows it.
type LoaderRef struct {
	Kind    string `json:"kind"`
	Version string `json:"version"` // display name, "47.3.0"
	ID      string `json:"id"`      // stored build id, "1.20.1-47.3.0"
}

// InstanceSummary is a Library card.
type InstanceSummary struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Art         instance.Art `json:"art"`
	Icon        string       `json:"icon,omitempty"` // sha256 of a picture, served at /tex/icon/<icon>.png
	Minecraft   string       `json:"minecraft"`
	Loader      *LoaderRef   `json:"loader,omitempty"`
	Kind        string       `json:"kind"` // vanilla | modded | modpack
	Group       string       `json:"group,omitempty"`
	Tags        []string     `json:"tags"`
	HasServers  bool         `json:"hasServers"`
	LastPlayed  int64        `json:"lastPlayed,omitempty"` // unix ms
	PlaytimeMin int          `json:"playtimeMin"`
	Running     bool         `json:"running"`
}

// MemoryRange is in MB.
type MemoryRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// InstanceView is everything an instance page shows.
type InstanceView struct {
	InstanceSummary
	Sandboxed         bool              `json:"sandboxed"`
	Counts            map[string]int    `json:"counts"`
	Java              string            `json:"java"`
	JavaAuto          string            `json:"javaAuto"` // what "Automatic" picks, e.g. "Java 21"
	MemoryMB          MemoryRange       `json:"memoryMB"`
	Created           int64             `json:"created"`
	Folder            string            `json:"folder"`
	Moved             bool              `json:"moved"` // the game folder is outside the data folder
	Notes             string            `json:"notes"`
	KeepLoaderUpdated bool              `json:"keepLoaderUpdated"`
	FollowLatest      bool              `json:"followLatest"`
	SyncTargets       int               `json:"syncTargets"`
	DiscordStatus     bool              `json:"discordStatus"`
	Settings          instance.Settings `json:"settings"`
	GPU               string            `json:"gpu"` // the dedicated GPU's name, Linux only
}

// Errors returned to the UI.
var (
	ErrBadInstanceID = errors.New("api: invalid instance id")
	ErrBusy          = errors.New("api: close the game first")
)

func (s *Instances) summary(in instance.Instance) InstanceSummary {
	v := InstanceSummary{
		ID: in.ID, Name: in.Name, Art: in.Art, Icon: in.Icon, Minecraft: in.Minecraft, Kind: "vanilla",
		Group: in.Group, Tags: in.Tags, PlaytimeMin: in.PlaytimeMin, Running: s.d.Running(in.ID),
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if in.Loader != "" {
		k := loader.Kind(in.Loader)
		v.Loader = &LoaderRef{Kind: in.Loader, Version: loader.Display(k, in.Minecraft, in.LoaderVersion), ID: in.LoaderVersion}
		v.Kind = "modded"
	}
	if in.LastPlayed != nil {
		v.LastPlayed = in.LastPlayed.UnixMilli()
	}
	return v
}

func (s *Instances) view(in instance.Instance) InstanceView {
	g := s.d.Settings.Get()
	eff := in.Resolve(g)
	auto := "Automatic"
	if e, err := s.d.Meta.Entry(s.ctx.Get(), in.Minecraft); err == nil {
		auto = fmt.Sprintf("Java %d", e.EstimatedJava())
	}
	java := auto + ", automatic"
	if eff.JavaPath != "" {
		java = eff.JavaPath
	}
	return InstanceView{
		InstanceSummary: s.summary(in),
		Sandboxed:       in.Sandbox, Java: java, JavaAuto: auto, MemoryMB: MemoryRange{Min: eff.MemoryMinMB, Max: eff.MemoryMaxMB},
		Created: in.Created.UnixMilli(), Folder: s.d.Store.GameDir(in.ID), Moved: in.Location != "", Notes: in.Notes,
		KeepLoaderUpdated: in.KeepLoaderUpdated, FollowLatest: in.FollowLatest,
		Counts:        map[string]int{"mods": 0, "resourcepacks": 0, "shaders": 0, "datapacks": 0, "worlds": 0, "servers": 0},
		DiscordStatus: g.Integrations.DiscordStatus, Settings: in.Settings, GPU: sysinfo.DedicatedGPU(),
	}
}

// changed tells every screen and returns the instance's new view.
func (s *Instances) changed(in instance.Instance, err error) (InstanceView, error) {
	if err != nil {
		return InstanceView{}, err
	}
	s.d.Bus.Emit(events.InstancesChanged, nil)
	return s.view(in), nil
}

func checkInstanceID(id string) error {
	if !instance.ValidID(id) {
		return ErrBadInstanceID
	}
	return nil
}

// List returns every instance, most recently played first.
func (s *Instances) List() []InstanceSummary {
	list := s.d.Store.List()
	out := make([]InstanceSummary, len(list))
	for i, in := range list {
		out[i] = s.summary(in)
	}
	return out
}

// Get returns one instance.
func (s *Instances) Get(id string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	in, err := s.d.Store.Get(id)
	if err != nil {
		return InstanceView{}, err
	}
	return s.view(in), nil
}

// Create makes a new instance after checking the version and loader build
// exist upstream. An empty loader version means the newest stable build.
func (s *Instances) Create(req instance.CreateRequest) (InstanceView, error) {
	ctx := s.ctx.Get()
	if _, err := s.d.Meta.Entry(ctx, req.Minecraft); err != nil {
		return InstanceView{}, err
	}
	if req.Loader == "none" {
		req.Loader = ""
	}
	if req.Loader != "" {
		v, err := s.loaderBuild(ctx, req.Loader, req.Minecraft, req.LoaderVersion)
		if err != nil {
			return InstanceView{}, err
		}
		req.LoaderVersion = v
	}
	g := s.d.Settings.Get().Java
	return s.changed(s.d.Store.Create(req, instance.JavaSettings{MemoryMinMB: g.MemoryMinMB, MemoryMaxMB: g.MemoryMaxMB, JVMArgs: g.JVMArgs}))
}

// loaderBuild checks kind's build id for mc upstream ("" picks the newest stable).
func (s *Instances) loaderBuild(ctx context.Context, kind, mc, id string) (string, error) {
	k, err := loader.ParseKind(kind)
	if err != nil {
		return "", err
	}
	if id == "" {
		v, err := s.d.Loaders.Latest(ctx, k, mc)
		return v.ID, err
	}
	if !loader.ValidVersionID(id) {
		return "", loader.ErrUnknownVersion
	}
	v, err := s.d.Loaders.Find(ctx, k, mc, id)
	return v.ID, err
}

// UpdateSettings replaces an instance's settings.
func (s *Instances) UpdateSettings(id string, st instance.Settings) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.SetSettings(id, st))
}

// Rename changes an instance's name (its folder stays).
func (s *Instances) Rename(id, name string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.Rename(id, name))
}

// SetArt sets the tile colour and letter.
func (s *Instances) SetArt(id string, art instance.Art) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.SetArt(id, art))
}

// PickIcon lets the user choose a picture in a native dialog (the path never
// comes from the page). A cancelled dialog changes nothing.
func (s *Instances) PickIcon(id string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	hash, err := s.chooseIcon()
	if err != nil || hash == "" {
		if err == nil {
			return s.Get(id)
		}
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.SetIcon(id, hash))
}

// SetIcon uses a picture already in the icon library ("" goes back to the tile).
func (s *Instances) SetIcon(id, hash string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	if hash != "" {
		if _, err := s.d.Icons.Path(hash); err != nil {
			return InstanceView{}, icons.ErrNotIcon
		}
	}
	return s.changed(s.d.Store.SetIcon(id, hash))
}

// ChooseIcon picks a picture for an instance that doesn't exist yet (New
// instance); it returns the icon's name, or "" when cancelled.
func (s *Instances) ChooseIcon() (string, error) { return s.chooseIcon() }

func (s *Instances) chooseIcon() (string, error) {
	path, err := wruntime.OpenFileDialog(s.ctx.Get(), wruntime.OpenDialogOptions{
		Title:   "Choose a picture",
		Filters: []wruntime.FileFilter{{DisplayName: "Pictures (*.png, *.jpg)", Pattern: "*.png;*.jpg;*.jpeg"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return s.d.Icons.Import(path)
}

// SetNotes stores an instance's notes.
func (s *Instances) SetNotes(id, notes string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.SetNotes(id, notes))
}

// SetTags replaces an instance's tags.
func (s *Instances) SetTags(id string, tags []string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.SetTags(id, tags))
}

// Groups lists the groups in their order.
func (s *Instances) Groups() []instance.Group { return s.d.Store.Groups() }

// SetGroup moves an instance into a group ("" for none).
func (s *Instances) SetGroup(id, group string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.SetGroup(id, group))
}

// CreateGroup adds an empty group.
func (s *Instances) CreateGroup(name string) (instance.Group, error) {
	g, err := s.d.Store.CreateGroup(name)
	if err == nil {
		s.d.Bus.Emit(events.InstancesChanged, nil)
	}
	return g, err
}

// RenameGroup renames a group and its members' group.
func (s *Instances) RenameGroup(from, to string) error {
	return s.groupsChanged(s.d.Store.RenameGroup(from, to))
}

// DeleteGroup removes a group; its instances become ungrouped.
func (s *Instances) DeleteGroup(name string) error {
	return s.groupsChanged(s.d.Store.DeleteGroup(name))
}

// SetGroupCollapsed remembers a group folded away.
func (s *Instances) SetGroupCollapsed(name string, collapsed bool) error {
	return s.groupsChanged(s.d.Store.SetGroupCollapsed(name, collapsed))
}

func (s *Instances) groupsChanged(err error) error {
	if err == nil {
		s.d.Bus.Emit(events.InstancesChanged, nil)
	}
	return err
}

// SetVersion moves an instance to another Minecraft version and loader
// build. Changing Minecraft with a loader keeps the loader on its newest
// stable build for the new version (Prism's update task), or fails when
// there is none yet. loader "" removes the loader.
func (s *Instances) SetVersion(id, minecraft, kind, loaderVersion string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	if s.d.Busy(id) {
		return InstanceView{}, ErrBusy
	}
	ctx := s.ctx.Get()
	if _, err := s.d.Meta.Entry(ctx, minecraft); err != nil {
		return InstanceView{}, err
	}
	if kind == "none" {
		kind = ""
	}
	if kind != "" {
		v, err := s.loaderBuild(ctx, kind, minecraft, loaderVersion)
		if err != nil {
			return InstanceView{}, err
		}
		loaderVersion = v
	} else {
		loaderVersion = ""
	}
	return s.changed(s.d.Store.SetVersion(id, minecraft, kind, loaderVersion))
}

// SetVersionOptions stores the Version tab's two switches.
func (s *Instances) SetVersionOptions(id string, keepLoaderUpdated, followLatest bool) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.SetVersionOptions(id, keepLoaderUpdated, followLatest))
}

// Trash moves an instance to the launcher's trash (restorable for 30 days).
func (s *Instances) Trash(id string) (instance.TrashEntry, error) {
	if err := checkInstanceID(id); err != nil {
		return instance.TrashEntry{}, err
	}
	if s.d.Busy(id) {
		return instance.TrashEntry{}, ErrBusy
	}
	e, err := s.d.Store.Trash(id, time.Now())
	if err == nil {
		s.d.Bus.Emit(events.InstancesChanged, nil)
	}
	return e, err
}

// TrashList lists the trash, newest first.
func (s *Instances) TrashList() []instance.TrashEntry {
	if l := s.d.Store.TrashList(); l != nil {
		return l
	}
	return []instance.TrashEntry{}
}

// Restore brings a trashed instance back and returns it.
func (s *Instances) Restore(key string) (InstanceView, error) {
	return s.changed(s.d.Store.Restore(key))
}

// Purge deletes a trashed instance for good.
func (s *Instances) Purge(key string) error {
	return s.groupsChanged(s.d.Store.Purge(key))
}

// Move lets the user pick a folder (native dialog) and moves the game folder
// there as a job; "" means cancelled. MoveDefault moves it back.
func (s *Instances) Move(id string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	parent, err := wruntime.OpenDirectoryDialog(s.ctx.Get(), wruntime.OpenDialogOptions{Title: "Choose where the game folder goes"})
	if err != nil || parent == "" {
		return "", err
	}
	return s.move(id, parent)
}

// MoveDefault moves a moved game folder back into the data folder.
func (s *Instances) MoveDefault(id string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	return s.move(id, "")
}

func (s *Instances) move(id, parent string) (string, error) {
	if s.d.Busy(id) {
		return "", ErrBusy
	}
	in, err := s.d.Store.Get(id)
	if err != nil {
		return "", err
	}
	plan, err := s.d.Store.PlanMove(id, parent)
	if err != nil {
		return "", err
	}
	spec := tasks.NewJob(tasks.Meta{Title: "Moving " + in.Name, Subtitle: plan.To, Art: tasks.Art{Background: in.Art.Background, Initial: in.Art.Initial}, Kind: "move"}).
		Step("Game folder", tasks.Count, tasks.Func("move the game folder", func(ctx context.Context) error {
			if s.d.Busy(id) {
				return ErrBusy
			}
			_, err := s.d.Store.Move(ctx, plan, nil)
			if err == nil {
				s.d.Bus.Emit(events.InstancesChanged, nil)
			}
			return err
		}))
	return s.d.Tasks.Submit(spec)
}

// OpenFolder shows the instance's game folder in the file manager.
func (s *Instances) OpenFolder(id string) error {
	dir, err := s.d.Store.GameDirSafe(id)
	if err != nil {
		return err
	}
	return procx.OpenFolder(dir)
}

// TotalPlaytime is the minutes played across every instance.
func (s *Instances) TotalPlaytime() int { return s.d.Store.TotalPlaytime() }
