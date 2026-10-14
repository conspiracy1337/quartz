package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/content"
	"quartz/internal/content/store"
	"quartz/internal/core/events"
	"quartz/internal/core/procx"
	"quartz/internal/core/sysinfo"
	"quartz/internal/core/tasks"
	"quartz/internal/follow"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/modpack"
	"quartz/internal/loader"
	"quartz/internal/meta"
	"quartz/internal/modplatform"
	"quartz/internal/settings"
	"quartz/internal/shortcuts"
	"quartz/internal/syncing"
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
	// Hosting reports a local server running the instance; nil in tests.
	Hosting func(id string) bool
	// Content counts items and moves mods along with version changes; nil in tests.
	Content *content.Service
	// Worlds and Servers count an instance's worlds and servers; nil in tests.
	Worlds  Counter
	Servers Counter
	// Syncing joins instances to shared targets; nil in tests.
	Syncing *syncing.Service
	// Shortcuts writes desktop and menu shortcuts; nil in tests.
	Shortcuts *shortcuts.Service
	// Follow offers moves to the newest release; nil in tests.
	Follow *follow.Checker
	// Opens shows toasts for work that ends after the call returns; nil in tests.
	Opens *Opens
	// Blobs holds picked client jars until an instance takes them; nil in tests.
	Blobs *store.Store
}

// Counter counts one kind of thing an instance has.
type Counter interface{ Count(id string) int }

// Instances exposes the instance list and everything done to one instance.
type Instances struct {
	ctx *Ctx
	d   InstanceDeps

	jarMu sync.Mutex
	jars  map[string]ClientJarPick // checked by ChooseClientJar, by sha1
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
	Created     int64        `json:"created"`              // unix ms
	PlaytimeMin int          `json:"playtimeMin"`
	Running     bool         `json:"running"`
	// ClientJar is the custom client jar it runs in place of vanilla's.
	ClientJar *ClientJarRef `json:"clientJar,omitempty"`
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
	JavaAuto          string            `json:"javaAuto"`      // what "Automatic" picks, e.g. "Java 21"
	SandboxPreset     string            `json:"sandboxPreset"` // standard | strict | custom
	MemoryMB          MemoryRange       `json:"memoryMB"`
	Folder            string            `json:"folder"`
	Moved             bool              `json:"moved"` // the game folder is outside the data folder
	Notes             string            `json:"notes"`
	KeepLoaderUpdated bool              `json:"keepLoaderUpdated"`
	FollowLatest      bool              `json:"followLatest"`
	SyncTargets       int               `json:"syncTargets"` // shared targets it joins that are on
	Sync              instance.Sync     `json:"sync"`
	DiscordStatus     bool              `json:"discordStatus"` // the instance's own switch
	Settings          instance.Settings `json:"settings"`
	GPU               string            `json:"gpu"` // the dedicated GPU's name, Linux only
	// Pack is the modpack the instance was installed from.
	Pack *PackView `json:"pack,omitempty"`
	// Switch reports a version change's mod job (SetVersion only).
	Switch *content.SwitchResult `json:"switch,omitempty"`
}

// PackView names an instance's modpack and whether it can change version.
type PackView struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Updatable bool   `json:"updatable"`
}

// Errors returned to the UI.
var (
	ErrBadInstanceID = errors.New("api: invalid instance id")
	ErrBusy          = errors.New("api: close the game first")
	ErrHosting       = errors.New("api: stop the server first")
	ErrContentBusy   = errors.New("api: a download for this instance is still running. Let it finish or cancel it in Downloads")
)

// guard refuses changes to files a game, a hosted server or a content job
// (install, update, version switch) is using.
func (s *Instances) guard(id string) error {
	if s.d.Hosting != nil && s.d.Hosting(id) {
		return ErrHosting
	}
	if s.d.Busy(id) {
		return ErrBusy
	}
	if s.d.Content != nil && s.d.Content.Busy(id) {
		return ErrContentBusy
	}
	return nil
}

func (s *Instances) summary(in instance.Instance) InstanceSummary {
	v := InstanceSummary{
		ID: in.ID, Name: in.Name, Art: in.Art, Icon: in.Icon, Minecraft: in.Minecraft, Kind: "vanilla",
		Group: in.Group, Tags: in.Tags, PlaytimeMin: in.PlaytimeMin, Running: s.d.Running(in.ID), Created: in.Created.UnixMilli(),
		HasServers: s.d.Servers != nil && s.d.Servers.Count(in.ID) > 0, ClientJar: clientJarRef(in),
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if in.Loader != "" {
		k := loader.Kind(in.Loader)
		v.Loader = &LoaderRef{Kind: in.Loader, Version: loader.Display(k, in.Minecraft, in.LoaderVersion), ID: in.LoaderVersion}
		v.Kind = "modded"
	}
	if in.Pack != nil {
		v.Kind = "modpack"
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
		Sandboxed:       in.Sandbox.On, SandboxPreset: string(in.Sandbox.Preset), Java: java, JavaAuto: auto, MemoryMB: MemoryRange{Min: eff.MemoryMinMB, Max: eff.MemoryMaxMB},
		Folder: s.d.Store.GameDir(in.ID), Moved: in.Location != "", Notes: in.Notes,
		KeepLoaderUpdated: in.KeepLoaderUpdated, FollowLatest: in.FollowLatest,
		Counts:        s.counts(in.ID),
		DiscordStatus: !in.HideDiscord, Settings: in.Settings, GPU: sysinfo.DedicatedGPU(),
		SyncTargets: s.syncTargets(in.ID), Sync: in.Sync, Pack: packView(in),
	}
}

func packView(in instance.Instance) *PackView {
	if in.Pack == nil {
		return nil
	}
	o := modpack.Origin{Source: in.Pack.Source, Project: in.Pack.Project}
	return &PackView{Name: in.Pack.Name, Version: in.Pack.VersionName, Updatable: o.Updatable() && !in.InPlace}
}

func (s *Instances) syncTargets(id string) int {
	if s.d.Syncing == nil {
		return 0
	}
	return len(s.d.Syncing.Joined(id))
}

// counts is how many items each content tab has.
func (s *Instances) counts(id string) map[string]int {
	out := map[string]int{"mods": 0, "resourcepacks": 0, "shaders": 0, "datapacks": 0, "worlds": 0, "servers": 0}
	if s.d.Content != nil {
		for k, n := range s.d.Content.Counts(id) {
			out[string(k)] = n
		}
	}
	if s.d.Worlds != nil {
		out["worlds"] = s.d.Worlds.Count(id)
	}
	if s.d.Servers != nil {
		out["servers"] = s.d.Servers.Count(id)
	}
	return out
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
	return asList(out)
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
	if len(req.Shares) > 32 {
		return InstanceView{}, fmt.Errorf("%w: shares", instance.ErrInvalidSettings)
	}
	var jar ClientJarPick
	if req.ClientJar != "" {
		if req.Loader != "" {
			return InstanceView{}, instance.ErrClientJarLocked
		}
		var err error
		if jar, err = s.pickedJar(ctx, req.ClientJar, req.Minecraft); err != nil {
			return InstanceView{}, err
		}
	}
	in, err := s.d.Store.Create(req)
	if err == nil && req.ClientJar != "" {
		in, err = s.attachJar(in, jar)
	}
	if err == nil && s.d.Syncing != nil && len(req.Shares) > 0 {
		// A new instance has nothing to merge in; a target that can't be
		// joined just stays off for it.
		_ = s.d.Syncing.JoinAll(ctx, in.ID, req.Shares)
		in, err = s.d.Store.Get(in.ID)
	}
	return s.changed(in, err)
}

// SetSync joins or leaves a shared target (see package syncing).
func (s *Instances) SetSync(id, target string, on bool) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	if !instance.ValidSyncID(target) || s.d.Syncing == nil {
		return InstanceView{}, errBadSync
	}
	var err error
	if on {
		err = s.d.Syncing.Join(s.ctx.Get(), id, target)
	} else {
		err = s.d.Syncing.Leave(s.ctx.Get(), id, target)
	}
	if err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.Get(id))
}

// SetSharedKeys turns the shared keybinds on or off for an instance.
func (s *Instances) SetSharedKeys(id string, on bool) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	if s.d.Syncing == nil {
		return InstanceView{}, errBadSync
	}
	if err := s.d.Syncing.SetKeys(id, on); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.Get(id))
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
	path, err := openFileDialog(s.ctx.Get(), wruntime.OpenDialogOptions{
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
func (s *Instances) Groups() []instance.Group { return asList(s.d.Store.Groups()) }

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
	if err := s.guard(id); err != nil {
		return InstanceView{}, err
	}
	ctx := s.ctx.Get()
	if _, err := s.d.Meta.Entry(ctx, minecraft); err != nil {
		return InstanceView{}, err
	}
	if kind == "none" {
		kind = ""
	}
	// Refused before the mods' job starts, not only when it applies.
	if in, err := s.d.Store.Get(id); err != nil {
		return InstanceView{}, err
	} else if in.ClientJar != nil && (minecraft != in.Minecraft || kind != "") {
		return InstanceView{}, instance.ErrClientJarLocked
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
	apply := func(context.Context) error {
		_, err := s.changed(s.d.Store.SetVersion(id, minecraft, kind, loaderVersion))
		return err
	}
	if s.d.Content == nil {
		if err := apply(ctx); err != nil {
			return InstanceView{}, err
		}
		return s.Get(id)
	}
	// Mods move with the version: a job updates them, then applies the change.
	res, err := s.d.Content.Switch(ctx, id, minecraft, kind, modplatform.ParseChannel(s.d.Settings.Get().Updates.ModChannel), apply)
	if err != nil {
		return InstanceView{}, err
	}
	v, err := s.Get(id)
	if err == nil && (res.Job != "" || len(res.Disabled) > 0 || len(res.Unknown) > 0) {
		v.Switch = &res
	}
	return v, err
}

// SetDiscord turns the instance's Discord status on or off (Overview).
func (s *Instances) SetDiscord(id string, on bool) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.SetDiscord(id, on))
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
	if err := s.guard(id); err != nil {
		return instance.TrashEntry{}, err
	}
	if s.d.Content != nil {
		s.d.Content.Forget(id)
	}
	in, _ := s.d.Store.Get(id)
	e, err := s.d.Store.Trash(id, time.Now())
	if err == nil {
		if s.d.Shortcuts != nil {
			s.d.Shortcuts.Hide(in)
		}
		s.d.Bus.Emit(events.InstancesChanged, nil)
	}
	return e, err
}

// TrashList lists the trash, newest first.
func (s *Instances) TrashList() []instance.TrashEntry {
	if l := s.d.Store.TrashList(); l != nil {
		return asList(l)
	}
	return []instance.TrashEntry{}
}

// Restore brings a trashed instance back, with its shortcuts, and returns it.
func (s *Instances) Restore(key string) (InstanceView, error) {
	in, err := s.d.Store.Restore(key)
	if err == nil && s.d.Shortcuts != nil {
		go s.d.Shortcuts.Restore(in)
	}
	return s.changed(in, err)
}

// Shortcut places: "desktop", "menu" or "both".
var shortcutPlaces = map[string][]shortcuts.Where{
	"desktop": {shortcuts.Desktop}, "menu": {shortcuts.Menu}, "both": {shortcuts.Desktop, shortcuts.Menu},
}

// ErrBadPlace is returned for an unknown shortcut place.
var ErrBadPlace = errors.New("api: choose the desktop, the app menu or both")

// CreateShortcut puts the instance on the desktop and/or in the app menu,
// optionally with a fixed account and a world or server to open.
func (s *Instances) CreateShortcut(id, where, accountID, kind, target string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	places, ok := shortcutPlaces[where]
	if !ok {
		return ErrBadPlace
	}
	if accountID != "" {
		if err := checkAccount(accountID); err != nil {
			return err
		}
	}
	into := instance.Into{Kind: kind, Target: target}
	if !into.Valid() {
		return instance.ErrInvalidSettings
	}
	if s.d.Shortcuts == nil {
		return shortcuts.ErrUnsupported
	}
	_, err := s.d.Shortcuts.Create(id, places, accountID, into)
	if err == nil {
		s.d.Bus.Emit(events.InstancesChanged, nil)
	}
	return err
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
	parent, err := openDirDialog(s.ctx.Get(), wruntime.OpenDialogOptions{Title: "Choose where the game folder goes"})
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
	if err := s.guard(id); err != nil {
		return "", err
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
			if err := s.guard(id); err != nil {
				return err
			}
			_, err := s.d.Store.Move(ctx, plan, nil)
			if err == nil {
				s.d.Bus.Emit(events.InstancesChanged, nil)
			}
			return err
		}))
	return s.d.Tasks.Submit(spec)
}

// Duplicate copies an instance into a new one ("<name> (copy)") as a job:
// worlds, options and content come along, mods linked from the store
// rather than stored twice; playtime and history start fresh.
func (s *Instances) Duplicate(id string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	if err := s.guard(id); err != nil {
		return "", err
	}
	in, err := s.d.Store.Get(id)
	if err != nil {
		return "", err
	}
	spec := tasks.NewJob(tasks.Meta{Title: "Duplicating " + in.Name, Subtitle: "A copy with its worlds and content", Art: tasks.Art{Background: in.Art.Background, Initial: in.Art.Initial}, Kind: "move"}).
		Step("Copying", tasks.Count, tasks.Func("copy the instance", func(ctx context.Context) error {
			if err := s.guard(id); err != nil {
				return err
			}
			var place func(rel, dst string) (bool, error)
			if s.d.Content != nil {
				// Fresh hashes first: a stored copy is only linked for a file
				// that is still exactly what was indexed.
				for _, k := range content.Kinds {
					if err := s.d.Content.Scan(ctx, id, k); err != nil {
						return err
					}
				}
				place = s.d.Content.Placer(id)
			}
			cp, err := s.d.Store.Duplicate(ctx, id, place, nil)
			if err != nil {
				return err
			}
			s.d.Bus.Emit(events.InstancesChanged, nil)
			if s.d.Opens != nil {
				s.d.Opens.Show(OpenTarget{Kind: OpenNotice, Subject: "Duplicated " + in.Name, Text: "as " + cp.Name, Instance: cp.ID})
			}
			return nil
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

// FollowOffers lists instances that follow the latest release and whose mods
// all support it (checked at start and every 6 hours).
func (s *Instances) FollowOffers() []follow.Offer {
	if s.d.Follow == nil {
		return []follow.Offer{}
	}
	return asList(s.d.Follow.Offers())
}

// TotalPlaytime is the minutes played across every instance.
func (s *Instances) TotalPlaytime() int { return s.d.Store.TotalPlaytime() }
