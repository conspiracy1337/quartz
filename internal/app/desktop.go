package app

import (
	"os"
	"sync"

	"quartz/internal/brand"
	"quartz/internal/core/events"
	"quartz/internal/desktop"
	"quartz/internal/settings"
)

// envRegister lets a development build register itself (for testing link
// handling); otherwise dev builds never touch the system's registration.
const envRegister = "QUARTZ_REGISTER"

// desktopWant is what the settings ask to be registered.
func desktopWant(s settings.Settings, exe string) desktop.Want {
	return desktop.Want{Exe: exe, Links: s.Integrations.HandleLinks, Autostart: s.General.StartWithComputer, Version: brand.Version}
}

// watchDesktop registers the launcher with the system now (in the
// background) and again when the link or autostart settings change.
func (a *app) watchDesktop() {
	if desktop.Dev() && os.Getenv(envRegister) == "" {
		return
	}
	exe, err := desktop.Executable()
	if err != nil {
		a.log.Warn("can't register with the system", "err", err)
		return
	}
	reg := desktop.New(a.paths.Root, a.log)
	// One worker applies the newest wanted state, so a quick on-off never
	// lands out of order.
	var (
		mu     sync.Mutex
		latest = desktopWant(a.settings.Get(), exe)
		kick   = make(chan struct{}, 1)
	)
	go func() {
		for range kick {
			mu.Lock()
			w := latest
			mu.Unlock()
			if err := reg.Sync(w); err != nil {
				a.log.Warn("registering with the system failed", "err", err)
			}
		}
	}()
	kick <- struct{}{}
	a.bus.Subscribe(events.SettingsChanged, func(p any) {
		next := desktopWant(p.(settings.Settings), exe)
		mu.Lock()
		changed := next != latest
		latest = next
		mu.Unlock()
		if changed {
			select {
			case kick <- struct{}{}:
			default:
			}
		}
	})
}
