package app

import (
	"sync"

	"quartz/internal/content"
	"quartz/internal/core/events"
	"quartz/internal/core/version"
	"quartz/internal/instance"
)

// lodMods build far-away terrain on many threads and allocate heavily
// (Distant Horizons, Voxy); ZGC keeps that from stuttering the game.
var lodMods = []struct{ id, name string }{{"distanthorizons", "Distant Horizons"}, {"voxy", "Voxy"}}

// zgcFrom is the first Minecraft that runs on a Java with ZGC (17; older
// versions use Java 8, which has none).
const zgcFrom = "1.17"

// watchLODMods switches an instance to ZGC when a LOD mod is installed,
// turned on or imported with it: only then, so a collector the player picks
// afterwards stays (see instance.PreferGC).
func (a *app) watchLODMods() {
	ids := make([]string, 0, len(lodMods))
	for _, m := range lodMods {
		ids = append(ids, m.id)
	}
	var mu sync.Mutex
	had := map[string]bool{}
	for _, in := range a.instances.List() {
		had[in.ID] = a.content.HasEnabledMod(in.ID, ids...)
	}
	a.bus.Subscribe(events.ContentChanged, func(p any) {
		c, ok := p.(content.Changed)
		if !ok || c.Kind != content.Mods {
			return
		}
		has := a.content.HasEnabledMod(c.InstanceID, ids...)
		mu.Lock()
		was := had[c.InstanceID]
		had[c.InstanceID] = has
		mu.Unlock()
		if has && !was {
			a.preferZGC(c.InstanceID)
		}
	})
}

func (a *app) preferZGC(id string) {
	g := a.settings.Get().Java
	changed := false
	in, err := a.instances.Update(id, func(in *instance.Instance) error {
		if version.Compare(in.Minecraft, zgcFrom) >= 0 {
			changed = instance.PreferGC(in, g, instance.ZGC)
		}
		return nil
	})
	if err != nil {
		a.log.Warn("can't switch to ZGC", "instance", id, "err", err)
		return
	}
	if !changed {
		return
	}
	mod := ""
	for _, m := range lodMods {
		if a.content.HasEnabledMod(id, m.id) {
			mod = m.name
			break
		}
	}
	a.content.Record(in.ID, content.Activity{Kind: "updated", Title: "Switched to ZGC for " + mod, Detail: "Java settings now use " + instance.ZGC})
	a.bus.Emit(events.InstancesChanged, nil)
}
