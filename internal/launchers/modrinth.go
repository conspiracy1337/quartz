package launchers

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"quartz/internal/core/sqlitero"
)

func modrinthRoots(e Env) []string {
	return []string{filepath.Join(e.Data, "ModrinthApp"), filepath.Join(e.Data, "com.modrinth.theseus")}
}

// readModrinth reads the Modrinth App's app.db: the newer schema keeps
// instances and their content sets apart, the older one has one profiles
// table. Profile folders live in profiles/ under the root, or under the
// custom folder its settings name.
func readModrinth(root string) ([]Source, error) {
	db, err := sqlitero.Open(filepath.Join(root, "app.db"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	dirs := []string{filepath.Join(root, "profiles")}
	if custom := firstString(db, "settings", "custom_dir"); custom != "" && filepath.IsAbs(custom) && filepath.Clean(custom) != filepath.Clean(root) {
		dirs = append([]string{filepath.Join(custom, "profiles")}, dirs...)
	}
	var rows []map[string]any
	if db.HasTable("instances") && db.HasTable("instance_content_sets") {
		rows, err = modrinthInstances(db)
	} else {
		rows, err = scanRows(db, "profiles", "path", []string{"icon_path", "game_version", "mod_loader", "mod_loader_version", "name",
			"last_played", "submitted_time_played", "recent_time_played", "override_mc_memory_max"})
	}
	if err != nil {
		return nil, err
	}
	var out []Source
	for _, r := range rows {
		if s, ok := modrinthSource(r, dirs); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// modrinthInstances joins instances to their applied content set.
func modrinthInstances(db *sqlitero.DB) ([]map[string]any, error) {
	sets, err := scanRows(db, "instance_content_sets", "id", []string{"game_version", "loader", "loader_version"})
	if err != nil {
		return nil, err
	}
	byID := map[any]map[string]any{}
	for _, s := range sets {
		byID[s["id"]] = s
	}
	rows, err := scanRows(db, "instances", "path", []string{"icon_path", "applied_content_set_id", "name", "last_played",
		"submitted_time_played", "recent_time_played", "override_mc_memory_max"})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if s := byID[r["applied_content_set_id"]]; s != nil {
			r["game_version"], r["mod_loader"], r["mod_loader_version"] = s["game_version"], s["loader"], s["loader_version"]
		}
	}
	return rows, nil
}

func modrinthSource(r map[string]any, dirs []string) (Source, bool) {
	folder := str(r["path"])
	if folder == "" || folder != filepath.Base(folder) || strings.HasPrefix(folder, ".") {
		return Source{}, false
	}
	var game string
	for _, d := range dirs {
		if p := filepath.Join(d, folder); isDir(p) {
			game = p
			break
		}
	}
	mc := str(r["game_version"])
	loader, ok := loaderOf(str(r["mod_loader"]))
	if game == "" || mc == "" || !ok {
		return Source{}, false
	}
	s := Source{Name: clean(str(r["name"]), folder), Minecraft: mc, GameDir: game, Skip: []string{"profile.json"}}
	if loader != "" {
		s.Loader, s.LoaderVersion = loader, str(r["mod_loader_version"])
	}
	if icon := str(r["icon_path"]); filepath.IsAbs(icon) && isFile(icon) {
		s.Icon = icon
	}
	s.PlaytimeMin = minutes(num(r["submitted_time_played"]) + num(r["recent_time_played"]))
	if t := num(r["last_played"]); t > 0 {
		lp := time.Unix(t, 0).UTC()
		if t > 1e12 { // milliseconds
			lp = time.UnixMilli(t).UTC()
		}
		s.LastPlayed = &lp
	}
	if mb := num(r["override_mc_memory_max"]); mb > 0 && mb < 1<<20 {
		s.MemoryMB = int(mb)
	}
	return s, true
}

// scanRows reads key plus whichever of the optional columns the table has
// (schemas differ between versions).
func scanRows(db *sqlitero.DB, table, key string, optional []string) ([]map[string]any, error) {
	have, err := db.Columns(table)
	if err != nil {
		return nil, err
	}
	cols := []string{key}
	for _, c := range optional {
		if slices.ContainsFunc(have, func(h string) bool { return strings.EqualFold(h, c) }) {
			cols = append(cols, c)
		}
	}
	var out []map[string]any
	err = db.Scan(table, cols, func(row []any) error {
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = row[i]
		}
		out = append(out, m)
		return nil
	})
	return out, err
}

func firstString(db *sqlitero.DB, table, col string) string {
	var v string
	_ = db.Scan(table, []string{col}, func(row []any) error {
		if v == "" {
			v = str(row[0])
		}
		return nil
	})
	return v
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}
	return ""
}

func num(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	}
	return 0
}
