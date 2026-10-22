package api

import (
	"quartz/internal/content"
	"quartz/internal/modpack"
)

// ModpacksDeps are what the modpack facade works with.
type ModpacksDeps struct {
	Packs *modpack.Service
	Busy  func(id string) bool
}

// Modpacks exposes pack updates: Home's alerts and the update review.
type Modpacks struct {
	ctx *Ctx
	d   ModpacksDeps
}

// NewModpacks returns the facade.
func NewModpacks(ctx *Ctx, d ModpacksDeps) *Modpacks { return &Modpacks{ctx: ctx, d: d} }

// Updates lists packs with a newer version (from the last check).
func (m *Modpacks) Updates() []modpack.Available { return asList(m.d.Packs.Updates()) }

// Versions lists the versions of an instance's pack, newest first.
func (m *Modpacks) Versions(instanceID string) ([]modpack.PackVersion, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	return asListErr(m.d.Packs.Versions(m.ctx.Get(), instanceID))
}

// Diff reviews moving an instance's pack to version ("" for the update the
// last check found).
func (m *Modpacks) Diff(instanceID, version string) (*modpack.Diff, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	return m.d.Packs.Diff(m.ctx.Get(), instanceID, version)
}

// Update starts the job moving an instance's pack to version ("" for the
// update the last check found), backing worlds up first when asked. A
// snapshot is taken before the files change.
func (m *Modpacks) Update(instanceID, version string, backupWorlds bool) (content.Started, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return content.Started{}, err
	}
	if m.d.Busy(instanceID) {
		return content.Started{}, ErrBusy
	}
	return m.d.Packs.Update(m.ctx.Get(), instanceID, version, backupWorlds)
}
