package app

import (
	"context"
	"net/url"
	"os"
	"time"

	"quartz/internal/api"
	"quartz/internal/brand"
	"quartz/internal/core/events"
	"quartz/internal/desktop"
	"quartz/internal/settings"
	"quartz/internal/update"
)

// Update checks: shortly after start, then twice a day, while automatic
// updates are on (timers, no polling).
const (
	updateFirst = 30 * time.Second
	updateEvery = 12 * time.Hour
)

// buildUpdater makes the self-updater (off for dev builds and builds
// without a key).
func (a *app) buildUpdater() {
	exe, _ := desktop.Executable()
	a.updater = update.New(update.Config{
		Repo: brand.UpdateRepo, PublicKey: brand.UpdatePublicKey, Version: brand.Version, Dir: a.paths.Update(), Exe: exe,
		HTTP: a.http, Tasks: a.tasks, Bus: a.bus, Log: a.log, BaseURL: updateBase(),
	})
}

// updateBase is the release server override, accepted only as https or a
// local test server.
func updateBase() string {
	v := os.Getenv(brand.EnvUpdateBase)
	u, err := url.Parse(v)
	if v == "" || err != nil {
		return ""
	}
	local := u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"
	if u.Scheme == "https" || (u.Scheme == "http" && local) {
		return v
	}
	return ""
}

// finishUpdate reports how the last update went, once.
func (a *app) finishUpdate() {
	out := update.Finish(a.paths.Update(), brand.Version)
	switch {
	case out.Updated:
		a.opens.Show(api.OpenTarget{Kind: api.OpenNotice, Subject: "Updated", Text: "to " + brand.Name + " " + out.Version})
	case out.Failed:
		a.opens.Show(api.OpenTarget{Kind: api.OpenError, Subject: "The update to " + out.Version + " didn't install", Text: out.Error})
	}
}

// scheduleUpdates checks now and then while automatic updates are on, and
// returns the stop function.
func (a *app) scheduleUpdates() func() {
	done := make(chan struct{})
	wake := make(chan struct{}, 1)
	a.bus.Subscribe(events.SettingsChanged, func(p any) {
		if p.(settings.Settings).Updates.AutoCheck {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
	})
	go func() {
		t := time.NewTimer(updateFirst)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-wake:
			case <-t.C:
			}
			if a.settings.Get().Updates.AutoCheck {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				_, _ = a.updater.Check(ctx)
				cancel()
			}
			t.Reset(updateEvery)
		}
	}()
	return func() { close(done) }
}

// installOnQuit swaps in an update downloaded but not installed yet ("Later").
func (a *app) installOnQuit() {
	if a.updater.Pending() {
		if err := a.updater.Install(); err != nil {
			a.log.Warn("installing the update on quit failed", "err", err)
		}
	}
}
