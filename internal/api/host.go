package api

import (
	"os"

	"quartz/internal/core/procx"
	"quartz/internal/gamelog"
	"quartz/internal/host"
	"quartz/internal/instance"
)

// Host runs an instance as a local server.
type Host struct {
	ctx *Ctx
	h   *host.Service
}

func NewHost(ctx *Ctx, h *host.Service) *Host { return &Host{ctx: ctx, h: h} }

// Setup lists the worlds and the options the host dialog starts from.
func (a *Host) Setup(id string) (host.Setup, error) {
	if err := checkInstanceID(id); err != nil {
		return host.Setup{}, err
	}
	return a.h.Setup(a.ctx.Get(), id)
}

// Mods lists the instance's mods and whether each runs on its server.
func (a *Host) Mods(id string) ([]host.Mod, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return asListErr(a.h.Mods(a.ctx.Get(), id))
}

// SaveMods keeps the Manage picks: mod key → runs on the server.
func (a *Host) SaveMods(id string, picks map[string]bool) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	return a.h.SaveMods(a.ctx.Get(), id, picks)
}

// Properties reads the server's server.properties for the editor.
func (a *Host) Properties(id string) (host.Properties, error) {
	if err := checkInstanceID(id); err != nil {
		return host.Properties{}, err
	}
	return a.h.Properties(id)
}

// SaveProperties writes the editor's server.properties.
func (a *Host) SaveProperties(id, text string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	return a.h.SaveProperties(id, text)
}

// OpenFolder opens the instance's server folder (the same one every time
// it is hosted), making it first when it was never hosted.
func (a *Host) OpenFolder(id string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	dir := a.h.Dir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return procx.OpenFolder(dir)
}

// Start hosts an instance; progress arrives as host:state events.
func (a *Host) Start(id string, o instance.Host) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if err := host.Check(o); err != nil {
		return err
	}
	return a.h.Start(id, o)
}

// Stop asks the server to save and quit (a second call forces it).
func (a *Host) Stop(id string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	return a.h.Stop(id)
}

// Command sends one line to the server's console.
func (a *Host) Command(id, line string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	return a.h.Command(id, line)
}

// Status lists the running server, if any.
func (a *Host) Status() []host.State { return asList(a.h.Status()) }

// Live returns the server's console (or its last one).
func (a *Host) Live(id string) ([]gamelog.Line, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return asListErr(a.h.Live(id), nil)
}
