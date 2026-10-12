package api

import (
	"quartz/internal/core/sysinfo"
	"quartz/internal/install"
	"quartz/internal/meta"
	"quartz/internal/mojang"
)

// Versions exposes Minecraft version metadata.
type Versions struct {
	ctx       *Ctx
	meta      *meta.Service
	installer *install.Installer
}

func NewVersions(ctx *Ctx, m *meta.Service, in *install.Installer) *Versions {
	return &Versions{ctx: ctx, meta: m, installer: in}
}

// Estimate is how many bytes the first launch of a version still downloads
// (files other instances already share are not counted).
func (v *Versions) Estimate(id string) (int64, error) {
	res, err := v.installer.Resolve(v.ctx.Get(), install.Target{Minecraft: id}, "", mojang.CurrentEnv(sysinfo.OSVersion()))
	if err != nil {
		return 0, err
	}
	return v.installer.Estimate(res)
}

// Minecraft returns the supported versions grouped for the two-step picker.
func (v *Versions) Minecraft() ([]mojang.Family, error) {
	return v.meta.Families(v.ctx.Get())
}
