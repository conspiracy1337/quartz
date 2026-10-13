package api

import (
	"quartz/internal/update"
)

// Updates is the launcher's self-update (Settings, Updates; the banner).
type Updates struct {
	ctx     *Ctx
	u       *update.Updater
	restart func() error
}

// NewUpdates returns the facade; restart starts the launcher again.
func NewUpdates(ctx *Ctx, u *update.Updater, restart func() error) *Updates {
	return &Updates{ctx: ctx, u: u, restart: restart}
}

// State is where the updater is.
func (up *Updates) State() update.State { return up.u.State() }

// Check looks for a newer release now (and downloads it).
func (up *Updates) Check() (update.State, error) { return up.u.Check(up.ctx.Get()) }

// Restart installs the downloaded update and restarts into it.
func (up *Updates) Restart() error {
	if err := up.u.Install(); err != nil {
		return err
	}
	return up.restart()
}
