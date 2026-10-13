package launchers

import (
	"encoding/json"
	"path/filepath"
)

func atLauncherRoots(e Env) []string {
	roots := []string{filepath.Join(e.Data, "ATLauncher"), filepath.Join(e.Data, "atlauncher")}
	if e.OS == "linux" {
		roots = append(roots, filepath.Join(e.Home, ".var", "app", "com.atlauncher.ATLauncher", "data"))
	}
	return roots
}

// readATLauncher reads ATLauncher's instances: each instance folder is its
// game folder, with instance.json beside the game's files and disabled mods
// kept in disabledmods/.
func readATLauncher(root string) ([]Source, error) {
	if !isFile(filepath.Join(root, "configs", "ATLauncher.json")) {
		return nil, nil
	}
	var out []Source
	for _, inst := range subdirs(filepath.Join(root, "instances")) {
		b, err := readCapped(filepath.Join(inst, "instance.json"))
		if err != nil {
			continue
		}
		var m struct {
			ID       string `json:"id"` // the Minecraft version
			Launcher struct {
				Name          string `json:"name"`
				MaximumMemory int    `json:"maximumMemory"`
				LoaderVersion *struct {
					Version    string `json:"version"`
					RawVersion string `json:"rawVersion"`
					Type       string `json:"type"`
				} `json:"loaderVersion"`
			} `json:"launcher"`
		}
		if json.Unmarshal(b, &m) != nil || m.ID == "" {
			continue
		}
		s := Source{Name: clean(m.Launcher.Name, filepath.Base(inst)), Minecraft: m.ID, GameDir: inst,
			Skip: []string{"instance.json", "instance.png"}, Disabled: "disabledmods"}
		if lv := m.Launcher.LoaderVersion; lv != nil {
			loader, ok := loaderOf(lv.Type)
			if !ok {
				continue
			}
			s.Loader, s.LoaderVersion = loader, lv.Version
			if lv.RawVersion != "" {
				s.LoaderVersion = lv.RawVersion
			}
		}
		if m.Launcher.MaximumMemory > 0 {
			s.MemoryMB = m.Launcher.MaximumMemory
		}
		if icon := filepath.Join(inst, "instance.png"); isFile(icon) {
			s.Icon = icon
		}
		out = append(out, s)
	}
	return out, nil
}
