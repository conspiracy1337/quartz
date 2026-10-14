package launchers

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"quartz/internal/modpack"
)

func prismRoots(e Env) []string {
	roots := []string{filepath.Join(e.Data, "PrismLauncher")}
	if e.OS == "linux" {
		roots = append(roots, filepath.Join(e.Home, ".var", "app", "org.prismlauncher.PrismLauncher", "data", "PrismLauncher"))
	}
	return roots
}

func multiMCRoots(e Env) []string {
	return []string{filepath.Join(e.Data, "multimc"), filepath.Join(e.Data, "MultiMC")}
}

func readPrism(root string) ([]Source, error)   { return readMMC(root, "prismlauncher.cfg") }
func readMultiMC(root string) ([]Source, error) { return readMMC(root, "multimc.cfg") }

// readMMC reads a Prism or MultiMC data folder: its config names the
// instances folder (InstanceDir, relative to the root unless absolute).
func readMMC(root, config string) ([]Source, error) {
	b, err := readCapped(filepath.Join(root, config))
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "instances")
	if v := iniValues(b)["InstanceDir"]; v != "" {
		if filepath.IsAbs(v) {
			dir = filepath.Clean(v)
		} else {
			dir = filepath.Join(root, filepath.FromSlash(v))
		}
	}
	var out []Source
	for _, inst := range subdirs(dir) {
		if s, ok := readMMCInstance(root, inst); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func readMMCInstance(root, inst string) (Source, bool) {
	cfgData, err := readCapped(filepath.Join(inst, "instance.cfg"))
	if err != nil {
		return Source{}, false
	}
	packData, err := readCapped(filepath.Join(inst, "mmc-pack.json"))
	if err != nil {
		return Source{}, false
	}
	var p modpack.Pack
	if err := p.ReadMMCPack(packData); err != nil {
		return Source{}, false
	}
	cfg := iniValues(cfgData)
	s := Source{Name: clean(cfg["name"], filepath.Base(inst)), Minecraft: p.Minecraft, Loader: p.Loader, LoaderVersion: p.LoaderVersion, Warnings: p.Warnings}
	for _, d := range []string{"minecraft", ".minecraft"} {
		if g := filepath.Join(inst, d); isDir(g) {
			s.GameDir = g
			break
		}
	}
	if s.GameDir == "" {
		return Source{}, false
	}
	if n, err := strconv.ParseInt(cfg["totalTimePlayed"], 10, 64); err == nil {
		s.PlaytimeMin = minutes(n)
	}
	if ms, err := strconv.ParseInt(cfg["lastLaunchTime"], 10, 64); err == nil && ms > 0 {
		t := time.UnixMilli(ms).UTC()
		s.LastPlayed = &t
	}
	if cfg["OverrideMemory"] == "true" {
		if mb, err := strconv.Atoi(cfg["MaxMemAlloc"]); err == nil && mb > 0 {
			s.MemoryMB = mb
		}
	}
	s.Icon = mmcIcon(root, inst, cfg["iconKey"])
	s.Managed = modpack.ManagedFromPrism(cfg)
	return s, true
}

// mmcIcon finds an instance's custom icon: the icons folder holds it under
// its key; the built-in ones ("default", "grass", …) have no file.
func mmcIcon(root, inst, key string) string {
	if key == "" || strings.ContainsAny(key, `/\`) || strings.HasPrefix(key, ".") {
		return ""
	}
	for _, dir := range []string{filepath.Join(root, "icons"), inst} {
		for _, ext := range []string{".png", ".jpg", ".jpeg", ""} {
			if p := filepath.Join(dir, key+ext); isFile(p) {
				return p
			}
		}
	}
	return ""
}

// iniValues reads Qt INI key=value lines (sections ignored; values may be
// quoted, and may themselves contain '=').
func iniValues(b []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '[' || line[0] == ';' || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(v[1 : len(v)-1])
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}
