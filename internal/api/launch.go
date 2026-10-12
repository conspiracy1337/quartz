package api

import (
	"quartz/internal/gamelog"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

// Launch starts and stops games.
type Launch struct {
	ctx *Ctx
	l   *launch.Launcher
}

func NewLaunch(ctx *Ctx, l *launch.Launcher) *Launch { return &Launch{ctx: ctx, l: l} }

func checkInstance(id string) error {
	if !instance.ValidID(id) {
		return ErrBadInstanceID
	}
	return nil
}

// Play starts an instance with the default account (accountID "") or, for
// "Launch as", another one. Progress arrives as launch:progress events.
func (a *Launch) Play(id, accountID string) error {
	if err := checkInstance(id); err != nil {
		return err
	}
	if accountID != "" {
		if err := checkAccount(accountID); err != nil {
			return err
		}
	}
	return a.l.Play(id, accountID)
}

// CancelLaunch stops a launch before the game starts.
func (a *Launch) CancelLaunch(id string) {
	if checkInstance(id) == nil {
		a.l.CancelLaunch(id)
	}
}

// Stop asks a running game to quit (a second call forces it).
func (a *Launch) Stop(id string) error {
	if err := checkInstance(id); err != nil {
		return err
	}
	return a.l.Stop(id)
}

// Status lists running and starting instances.
func (a *Launch) Status() launch.Status { return a.l.Status() }

// Repair re-verifies an instance's game files; returns the job id ("" if all fine).
func (a *Launch) Repair(id string) (string, error) {
	if err := checkInstance(id); err != nil {
		return "", err
	}
	return a.l.Repair(a.ctx.Get(), id)
}

// Logs exposes the live console and launch history.
type Logs struct{ l *launch.Launcher }

func NewLogs(l *launch.Launcher) *Logs { return &Logs{l: l} }

// Live returns the console of the running (or last) run.
func (a *Logs) Live(id string) ([]gamelog.Line, error) {
	if err := checkInstance(id); err != nil {
		return nil, err
	}
	return a.l.Live(id), nil
}

// Runs returns the launch history, newest first.
func (a *Logs) Runs(id string) ([]launch.Record, error) {
	if err := checkInstance(id); err != nil {
		return nil, err
	}
	return a.l.Runs(id), nil
}
