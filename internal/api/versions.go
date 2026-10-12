package api

import (
	"quartz/internal/meta"
	"quartz/internal/mojang"
)

// Versions exposes Minecraft version metadata.
type Versions struct {
	ctx  *Ctx
	meta *meta.Service
}

func NewVersions(ctx *Ctx, m *meta.Service) *Versions { return &Versions{ctx: ctx, meta: m} }

// Minecraft returns the supported versions grouped for the two-step picker.
func (v *Versions) Minecraft() ([]mojang.Family, error) {
	return v.meta.Families(v.ctx.Get())
}
