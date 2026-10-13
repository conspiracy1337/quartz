package api

import (
	"quartz/internal/home"
	"quartz/internal/screenshots"
	"quartz/internal/servers"
)

// HomeDeps are what Home's facade reads.
type HomeDeps struct {
	Home        *home.Service
	Servers     *servers.Service
	Screenshots *screenshots.Service
}

// Home serves the dashboard's widgets.
type Home struct {
	ctx *Ctx
	d   HomeDeps
}

func NewHome(ctx *Ctx, d HomeDeps) *Home { return &Home{ctx: ctx, d: d} }

// Week is this week's play time per day, Monday first.
func (h *Home) Week() []home.Day { return h.d.Home.Week() }

// News is the latest Java Edition article (nil without one).
func (h *Home) News() (*home.News, error) { return h.d.Home.News(h.ctx.Get()) }

// Servers merges every instance's servers and pings them (force skips the
// one-minute cache).
func (h *Home) Servers(force bool) ([]servers.Merged, error) {
	return h.d.Servers.Home(h.ctx.Get(), force)
}

// RecentShots is the newest screenshots across instances.
func (h *Home) RecentShots() []screenshots.Shot { return h.d.Screenshots.Recent(6) }

// Disk is the data drive's free space.
func (h *Home) Disk() home.Disk { return h.d.Home.Disk() }
