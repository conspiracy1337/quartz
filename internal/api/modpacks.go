package api

import (
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
func (m *Modpacks) Updates() []modpack.Available { return m.d.Packs.Updates() }

// Diff reviews an instance's pack update.
func (m *Modpacks) Diff(instanceID string) (*modpack.Diff, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	return m.d.Packs.Diff(m.ctx.Get(), instanceID)
}

// Update starts the pack update job, backing worlds up first when asked.
func (m *Modpacks) Update(instanceID string, backupWorlds bool) (string, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return "", err
	}
	if m.d.Busy(instanceID) {
		return "", ErrBusy
	}
	return m.d.Packs.Update(m.ctx.Get(), instanceID, backupWorlds)
}
