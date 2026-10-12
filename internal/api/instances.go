package api

import (
	"errors"
	"fmt"

	"quartz/internal/core/events"
	"quartz/internal/core/procx"
	"quartz/internal/instance"
	"quartz/internal/meta"
	"quartz/internal/settings"
)

// Instances exposes the instance list and each instance's settings.
type Instances struct {
	ctx      *Ctx
	store    *instance.Store
	meta     *meta.Service
	settings *settings.Store
	bus      *events.Bus
	running  func(id string) bool
}

func NewInstances(ctx *Ctx, s *instance.Store, m *meta.Service, st *settings.Store, bus *events.Bus, running func(string) bool) *Instances {
	return &Instances{ctx: ctx, store: s, meta: m, settings: st, bus: bus, running: running}
}

// InstanceSummary is a Library card.
type InstanceSummary struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Art         instance.Art `json:"art"`
	Minecraft   string       `json:"minecraft"`
	Kind        string       `json:"kind"` // vanilla | modded | modpack
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
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Art           instance.Art      `json:"art"`
	Minecraft     string            `json:"minecraft"`
	Kind          string            `json:"kind"`
	HasServers    bool              `json:"hasServers"`
	LastPlayed    int64             `json:"lastPlayed,omitempty"`
	PlaytimeMin   int               `json:"playtimeMin"`
	Running       bool              `json:"running"`
	Sandboxed     bool              `json:"sandboxed"`
	Counts        map[string]int    `json:"counts"`
	Java          string            `json:"java"`
	MemoryMB      MemoryRange       `json:"memoryMB"`
	Created       int64             `json:"created"`
	Folder        string            `json:"folder"`
	SyncTargets   int               `json:"syncTargets"`
	DiscordStatus bool              `json:"discordStatus"`
	Settings      instance.Settings `json:"settings"`
}

// ErrBadInstanceID is returned for ids that cannot name an instance.
var ErrBadInstanceID = errors.New("api: invalid instance id")

func (s *Instances) summary(in instance.Instance) InstanceSummary {
	v := InstanceSummary{
		ID: in.ID, Name: in.Name, Art: in.Art, Minecraft: in.Minecraft, Kind: "vanilla",
		PlaytimeMin: in.PlaytimeMin, Running: s.running(in.ID),
	}
	if in.LastPlayed != nil {
		v.LastPlayed = in.LastPlayed.UnixMilli()
	}
	return v
}

func (s *Instances) view(in instance.Instance) InstanceView {
	sum := s.summary(in)
	g := s.settings.Get()
	mem := MemoryRange{Min: g.Java.MemoryMinMB, Max: g.Java.MemoryMaxMB}
	java := "Automatic"
	if j := in.Settings.Java; j.Override {
		mem = MemoryRange{Min: j.MemoryMinMB, Max: j.MemoryMaxMB}
		if j.JavaPath != "" {
			java = j.JavaPath
		}
	}
	if java == "Automatic" {
		if e, err := s.meta.Entry(s.ctx.Get(), in.Minecraft); err == nil {
			java = fmt.Sprintf("Java %d, automatic", e.EstimatedJava())
		}
	}
	return InstanceView{
		ID: sum.ID, Name: sum.Name, Art: sum.Art, Minecraft: sum.Minecraft, Kind: sum.Kind, HasServers: sum.HasServers,
		LastPlayed: sum.LastPlayed, PlaytimeMin: sum.PlaytimeMin, Running: sum.Running,
		Sandboxed: in.Sandbox, Java: java, MemoryMB: mem, Created: in.Created.UnixMilli(), Folder: s.store.GameDir(in.ID),
		Counts:        map[string]int{"mods": 0, "resourcepacks": 0, "shaders": 0, "datapacks": 0, "worlds": 0, "servers": 0},
		DiscordStatus: g.Integrations.DiscordStatus, Settings: in.Settings,
	}
}

// List returns every instance, most recently played first.
func (s *Instances) List() []InstanceSummary {
	list := s.store.List()
	out := make([]InstanceSummary, len(list))
	for i, in := range list {
		out[i] = s.summary(in)
	}
	return out
}

// Get returns one instance.
func (s *Instances) Get(id string) (InstanceView, error) {
	if !instance.ValidID(id) {
		return InstanceView{}, ErrBadInstanceID
	}
	in, err := s.store.Get(id)
	if err != nil {
		return InstanceView{}, err
	}
	return s.view(in), nil
}

// Create makes a new instance after checking the version exists.
func (s *Instances) Create(req instance.CreateRequest) (InstanceView, error) {
	if _, err := s.meta.Entry(s.ctx.Get(), req.Minecraft); err != nil {
		return InstanceView{}, err
	}
	g := s.settings.Get().Java
	in, err := s.store.Create(req, instance.JavaSettings{MemoryMinMB: g.MemoryMinMB, MemoryMaxMB: g.MemoryMaxMB, JVMArgs: g.JVMArgs})
	if err != nil {
		return InstanceView{}, err
	}
	s.bus.Emit(events.InstancesChanged, nil)
	return s.view(in), nil
}

// UpdateSettings replaces an instance's settings.
func (s *Instances) UpdateSettings(id string, st instance.Settings) (InstanceView, error) {
	if !instance.ValidID(id) {
		return InstanceView{}, ErrBadInstanceID
	}
	in, err := s.store.SetSettings(id, st)
	if err != nil {
		return InstanceView{}, err
	}
	s.bus.Emit(events.InstancesChanged, nil)
	return s.view(in), nil
}

// OpenFolder shows the instance's game folder in the file manager.
func (s *Instances) OpenFolder(id string) error {
	dir, err := s.store.GameDirSafe(id)
	if err != nil {
		return err
	}
	if _, err := s.store.Get(id); err != nil {
		return err
	}
	return procx.OpenFolder(dir)
}
