package launchers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"quartz/internal/modpack"
)

func curseForgeRoots(e Env) []string {
	return []string{filepath.Join(e.Home, "Documents", "curseforge", "minecraft"), filepath.Join(e.Home, "curseforge", "minecraft")}
}

// curseForgeLoaders maps baseModLoader.type to our loader ids.
var curseForgeLoaders = map[int]string{1: "forge", 4: "fabric", 5: "quilt", 6: "neoforge"}

// readCurseForge reads the CurseForge app's Instances folder: each instance
// is its own game folder with a minecraftinstance.json.
func readCurseForge(root string) ([]Source, error) {
	var out []Source
	for _, inst := range subdirs(filepath.Join(root, "Instances")) {
		b, err := readCapped(filepath.Join(inst, "minecraftinstance.json"))
		if err != nil {
			continue
		}
		var m struct {
			Name          string `json:"name"`
			GameVersion   string `json:"gameVersion"`
			BaseModLoader *struct {
				Type         int    `json:"type"`
				ForgeVersion string `json:"forgeVersion"`
			} `json:"baseModLoader"`
			LastPlayed       string `json:"lastPlayed"`
			IsMemoryOverride bool   `json:"isMemoryOverride"`
			AllocatedMemory  int    `json:"allocatedMemory"`
			// The pack the instance was installed from (0 for none).
			ProjectID        int64 `json:"projectID"`
			FileID           int64 `json:"fileID"`
			InstalledModpack *struct {
				AddonID       int64 `json:"addonID"`
				InstalledFile *struct {
					ID          int64  `json:"id"`
					DisplayName string `json:"displayName"`
				} `json:"installedFile"`
			} `json:"installedModpack"`
			Manifest *struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"manifest"`
		}
		if json.Unmarshal(b, &m) != nil || m.GameVersion == "" {
			continue
		}
		s := Source{Name: clean(m.Name, filepath.Base(inst)), Minecraft: m.GameVersion, GameDir: inst,
			Skip: []string{"minecraftinstance.json", "profileImage", "Install"}}
		if l := m.BaseModLoader; l != nil && l.Type != 0 {
			loader, ok := curseForgeLoaders[l.Type]
			if !ok {
				continue
			}
			s.Loader, s.LoaderVersion = loader, l.ForgeVersion
		}
		if t, err := time.Parse(time.RFC3339, m.LastPlayed); err == nil && t.Year() > 2000 {
			t = t.UTC()
			s.LastPlayed = &t
		}
		if m.IsMemoryOverride && m.AllocatedMemory > 0 {
			s.MemoryMB = m.AllocatedMemory
		}
		s.Icon = firstFile(filepath.Join(inst, "profileImage"))
		project, file := m.ProjectID, m.FileID
		if im := m.InstalledModpack; im != nil && im.InstalledFile != nil && project == 0 {
			project, file = im.AddonID, im.InstalledFile.ID
		}
		if project > 0 && file > 0 {
			s.Managed = &modpack.Managed{Origin: modpack.Origin{Source: "curseforge", Project: strconv.FormatInt(project, 10), Version: strconv.FormatInt(file, 10)}, Name: s.Name}
			if mf := m.Manifest; mf != nil {
				s.Managed.Name, s.Managed.VersionName = clean(mf.Name, s.Name), clean(mf.Version, "")
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// firstFile is the first regular file in dir by name ("" for none).
func firstFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name() < entries[b].Name() })
	for _, e := range entries {
		if p := filepath.Join(dir, e.Name()); e.Type().IsRegular() && isFile(p) {
			return p
		}
	}
	return ""
}
