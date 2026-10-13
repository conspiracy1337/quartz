package api

import (
	"quartz/internal/crash"
	"quartz/internal/gamelog"
	"quartz/internal/instance"
	"quartz/internal/launch"
	"quartz/internal/logfiles"
)

// Launch starts and stops games.
type Launch struct {
	ctx *Ctx
	l   *launch.Launcher
}

func NewLaunch(ctx *Ctx, l *launch.Launcher) *Launch { return &Launch{ctx: ctx, l: l} }

// Play starts an instance with the default account (accountID "") or, for
// "Launch as", another one. Progress arrives as launch:progress events.
func (a *Launch) Play(id, accountID string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if accountID != "" {
		if err := checkAccount(accountID); err != nil {
			return err
		}
	}
	return a.l.Play(id, accountID, instance.Into{})
}

// PlayInto starts an instance straight into a world (kind "world", its
// folder) or a server (kind "server", its address).
func (a *Launch) PlayInto(id, accountID, kind, target string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if accountID != "" {
		if err := checkAccount(accountID); err != nil {
			return err
		}
	}
	return a.l.Play(id, accountID, instance.Into{Kind: kind, Target: target})
}

// CancelLaunch stops a launch before the game starts.
func (a *Launch) CancelLaunch(id string) {
	if checkInstanceID(id) == nil {
		a.l.CancelLaunch(id)
	}
}

// Stop asks a running game to quit (a second call forces it).
func (a *Launch) Stop(id string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	return a.l.Stop(id)
}

// Status lists running and starting instances.
func (a *Launch) Status() launch.Status { return a.l.Status() }

// Repair re-verifies an instance's game files; returns the job id ("" if all fine).
func (a *Launch) Repair(id string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	return a.l.Repair(a.ctx.Get(), id)
}

// Logs exposes the live console, launch history, log files and crashes.
type Logs struct {
	ctx   *Ctx
	l     *launch.Launcher
	files *logfiles.Service
	crash *crash.Service
}

func NewLogs(ctx *Ctx, l *launch.Launcher, files *logfiles.Service, c *crash.Service) *Logs {
	return &Logs{ctx: ctx, l: l, files: files, crash: c}
}

func checkRun(id, runID string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if !launch.ValidRunID(runID) {
		return launch.ErrNoRun
	}
	return nil
}

// Live returns the console of the running (or last) run.
func (a *Logs) Live(id string) ([]gamelog.Line, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return a.l.Live(id), nil
}

// Runs returns the launch history, newest first.
func (a *Logs) Runs(id string) ([]launch.Record, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return a.l.Runs(id), nil
}

// Files lists the logs and crash reports.
func (a *Logs) Files(id string) ([]logfiles.File, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return a.files.Files(id), nil
}

// Text returns a log or crash report, redacted.
func (a *Logs) Text(id, name string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	return a.files.Text(id, name)
}

// RunText returns a file saved with a run: "log", "cmd" or "crash".
func (a *Logs) RunText(id, runID, kind string) (string, error) {
	if err := checkRun(id, runID); err != nil {
		return "", err
	}
	return a.files.RunText(id, runID, kind)
}

// Crash explains a run that crashed.
func (a *Logs) Crash(id, runID string) (crash.Info, error) {
	if err := checkRun(id, runID); err != nil {
		return crash.Info{}, err
	}
	return a.crash.ForRun(a.ctx.Get(), id, runID)
}

// CrashFile explains a report in the crash-reports folder.
func (a *Logs) CrashFile(id, name string) (crash.Info, error) {
	if err := checkInstanceID(id); err != nil {
		return crash.Info{}, err
	}
	return a.crash.ForFile(a.ctx.Get(), id, name)
}

// UploadFile sends a log file to mclo.gs and returns its page.
func (a *Logs) UploadFile(id, name string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	return a.files.UploadFile(a.ctx.Get(), id, name)
}

// UploadRun sends what a run saved (its crash report, else its log) to
// mclo.gs; runID "" is the latest run.
func (a *Logs) UploadRun(id, runID string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	if runID != "" && !launch.ValidRunID(runID) {
		return "", launch.ErrNoRun
	}
	return a.files.UploadRun(a.ctx.Get(), id, runID)
}

// DebugInfo is the text "Copy debug info" copies; runID "" is the latest run.
func (a *Logs) DebugInfo(id, runID string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	if runID != "" && !launch.ValidRunID(runID) {
		return "", launch.ErrNoRun
	}
	return a.files.DebugInfo(a.ctx.Get(), id, runID)
}

// OldSize is what "Delete old logs" frees; id "" counts every instance.
func (a *Logs) OldSize(id string) (int64, error) {
	if id != "" {
		if err := checkInstanceID(id); err != nil {
			return 0, err
		}
	}
	return a.files.OldSize(id), nil
}

// DeleteOld deletes compressed logs older than 30 days; id "" does every instance.
func (a *Logs) DeleteOld(id string) (int64, error) {
	if id != "" {
		if err := checkInstanceID(id); err != nil {
			return 0, err
		}
	}
	return a.files.DeleteOld(id), nil
}
