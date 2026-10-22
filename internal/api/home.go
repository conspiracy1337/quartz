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
func (h *Home) Week() []home.Day { return asList(h.d.Home.Week()) }

// News is the latest Java Edition article, then the launcher's own posts.
func (h *Home) News() ([]home.News, error) { return asListErr(h.d.Home.News(h.ctx.Get())) }

// Servers merges every instance's servers and pings them (force skips the
// one-minute cache).
func (h *Home) Servers(force bool) ([]servers.Merged, error) {
	return asListErr(h.d.Servers.Home(h.ctx.Get(), force))
}

// RemoveServers takes Home's rows (by merged ID) out of every instance that
// lists them; each part comes back with Servers.Undo on its instance.
func (h *Home) RemoveServers(keys []string) ([]servers.Removed, error) {
	return asListErr(h.d.Servers.RemoveEverywhere(keys))
}

// EditServer changes Home's row (by merged ID) in every instance that lists
// it and returns the row's ID after the edit.
func (h *Home) EditServer(key string, e servers.Entry) (string, error) {
	return h.d.Servers.EditEverywhere(key, e)
}

// RecentShots is the newest screenshots across instances.
func (h *Home) RecentShots() []screenshots.Shot { return asList(h.d.Screenshots.Recent(6)) }

// FolderProblem is what is wrong with where the data folder is ("bang",
// "temp"), or "".
func (h *Home) FolderProblem() string { return h.d.Home.FolderProblem() }

// Disk is the data drive's free space.
func (h *Home) Disk() home.Disk { return h.d.Home.Disk() }
