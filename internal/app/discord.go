package app

import (
	"time"

	"quartz/internal/brand"
	"quartz/internal/core/events"
	"quartz/internal/discord"
	"quartz/internal/launch"
)

// watchDiscord shows running games in Discord when both the global switch
// (Integrations) and the instance's own (Overview) are on.
func (a *app) watchDiscord() {
	enabled := func(id string) bool {
		if !a.settings.Get().Integrations.DiscordStatus {
			return false
		}
		in, err := a.instances.Get(id)
		return err == nil && !in.HideDiscord
	}
	a.presence = discord.NewPresence(brand.DiscordID(), enabled, a.log)
	if !a.presence.Ready() {
		a.log.Info("Discord status is not set up in this build", "env", brand.EnvDiscordClientID)
		return
	}
	a.bus.Subscribe(events.GameState, func(p any) {
		st, ok := p.(launch.State)
		if !ok {
			return
		}
		switch st.State {
		case "running":
			in, err := a.instances.Get(st.InstanceID)
			if err != nil {
				return
			}
			a.presence.Started(st.RunID, discord.Game{Instance: in.ID, Name: in.Name, Minecraft: in.Minecraft, Loader: in.Loader, Started: time.Now()})
		case "exited":
			a.presence.Exited(st.RunID)
		}
	})
	refresh := func(any) { a.presence.Refresh() }
	a.bus.Subscribe(events.SettingsChanged, refresh)
	a.bus.Subscribe(events.InstancesChanged, refresh)
}
