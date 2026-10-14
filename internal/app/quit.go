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

// beforeClose keeps the launcher open while downloads are unfinished (they
// would stop and need a retry): the UI says so and offers to quit anyway.
// Closing again while it says so quits. Quits the launcher asks for itself
// (restart, update, "Close" when a game starts) pass through.
func (a *app) beforeClose(context.Context) (prevent bool) {
	if a.forceQuit.Load() {
		return false
	}
	n := len(a.tasks.Snapshot().Active)
	now := time.Now()
	if n == 0 || now.Sub(time.Unix(0, a.quitAsked.Swap(now.UnixNano()))) < quitAgain {
		return false
	}
	subject, text := strconv.Itoa(n)+" downloads", "aren't finished yet"
	if n == 1 {
		subject, text = "1 download", "isn't finished yet"
	}
	a.opens.Show(api.OpenTarget{Kind: api.OpenQuitBlocked, Subject: subject, Text: text})
	return true
}

// quit closes the launcher without asking.
func (a *app) quit() {
	a.forceQuit.Store(true)
	wruntime.Quit(a.ctx.Get())
}
