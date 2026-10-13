package api

import (
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

// Mods counts the mods a server runs and names the client-only ones.
func (a *Host) Mods(id string) (host.Mods, error) {
	if err := checkInstanceID(id); err != nil {
		return host.Mods{}, err
	}
	return a.h.Mods(a.ctx.Get(), id)
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
