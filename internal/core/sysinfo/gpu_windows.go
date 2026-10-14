package sysinfo

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// DedicatedGPU isn't named on Windows: Windows itself picks its
// high-performance GPU (see PreferGPU).
func DedicatedGPU() string { return "" }

// OffloadEnv is Linux's PRIME offload.
func OffloadEnv() []string { return nil }

// gpuPreferences is where Settings → Display → Graphics keeps each program's
// choice; "GpuPreference=2;" is High performance.
const gpuPreferences = `Software\Microsoft\DirectX\UserGpuPreferences`

// PreferGPU asks Windows to run exe on its high-performance GPU (XMCL's
// way), unless the player already chose one for it in Windows' settings.
// The key is the user's own, so no elevation is needed.
func PreferGPU(exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, gpuPreferences, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if _, _, err := k.GetStringValue(exe); !errors.Is(err, registry.ErrNotExist) {
		return err // set already (or unreadable): leave it
	}
	return k.SetStringValue(exe, "GpuPreference=2;")
}
