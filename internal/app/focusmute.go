package app

import (
	"quartz/internal/core/events"
	"quartz/internal/focusmute"
)

// watchFocus mutes games while their window isn't focused (General →
// "Mute the game when it isn't focused"; Linux only). It runs only while
// the setting is on and a game runs.
func (a *app) watchFocus() {
	if !focusmute.Available() {
		return
	}
	w := focusmute.New(focusmute.Config{
		Games:   a.launcher.Games,
		Enabled: func() bool { return a.settings.Get().General.MuteUnfocused },
		Log:     a.log,
	})
	update := func(any) { w.Update() }
	a.bus.Subscribe(events.GameState, update)
	a.bus.Subscribe(events.SettingsChanged, update)
	w.Update() // games adopted at startup
}
