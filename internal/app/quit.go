package app

import (
	"context"
	"strconv"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/api"
)

// quitAgain is how long a second close request quits anyway: as long as the
// toast that asked stays up.
const quitAgain = 8 * time.Second

// beforeClose keeps the launcher open while it still has work running: a
// game or hosted server (quitting ends a sandboxed one with the launcher, and
// stops tracking any other) or unfinished downloads (they would stop and need
// a retry). The UI says so and offers to quit anyway; closing again while it
// says so quits. Quits the launcher asks for itself (restart, update) pass
// through.
func (a *app) beforeClose(context.Context) (prevent bool) {
	if a.forceQuit.Load() {
		return false
	}
	st := a.launcher.Status()
	subject, text, busy := closeBlocker(len(st.Running)+len(st.Launching), len(a.host.Status()), len(a.tasks.Snapshot().Active))
	now := time.Now()
	if !busy || now.Sub(time.Unix(0, a.quitAsked.Swap(now.UnixNano()))) < quitAgain {
		return false
	}
	a.opens.Show(api.OpenTarget{Kind: api.OpenQuitBlocked, Subject: subject, Text: text})
	return true
}

// closeBlocker words what closing would cut short, most important first;
// busy is false when nothing would.
func closeBlocker(games, servers, downloads int) (subject, text string, busy bool) {
	switch {
	case games == 1:
		return "1 game", "is still running", true
	case games > 1:
		return strconv.Itoa(games) + " games", "are still running", true
	case servers > 0:
		return "Your server", "is still running", true
	case downloads == 1:
		return "1 download", "isn't finished yet", true
	case downloads > 1:
		return strconv.Itoa(downloads) + " downloads", "aren't finished yet", true
	}
	return "", "", false
}

// quit closes the launcher without asking.
func (a *app) quit() {
	a.forceQuit.Store(true)
	wruntime.Quit(a.ctx.Get())
}
