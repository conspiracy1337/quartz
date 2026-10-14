package host

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"quartz/internal/core/fsx"
	"quartz/internal/core/nbt"
	"quartz/internal/core/version"
	"quartz/internal/worlds"
)

// A singleplayer world hosted on a dedicated server keeps its own rules, as
// Open to LAN does. The server takes the difficulty, hardcore and the game
// mode for new players from server.properties instead, and forces that
// difficulty onto the world on every start, so they are written from
// level.dat each time (in-game changes are saved there). A world with
// cheats on makes its host an operator.

var (
	difficulties = []string{"peaceful", "easy", "normal", "hard"}
	gameModes    = []string{"survival", "creative", "adventure", "spectator"}
)

// worldRules returns the server.properties values an existing world sets
// and whether it allows cheats; nil for a world that doesn't exist yet.
func worldRules(world, mc string) (map[string]string, bool) {
	data := worlds.LevelData(world)
	if data == nil {
		return nil, false
	}
	named := version.Compare(mc, "1.14") >= 0 // older servers read numbers only
	value := func(names []string, t *nbt.Tag) (string, bool) {
		n := int64(-1)
		switch {
		case t == nil:
		case t.Kind == nbt.String: // a name, in newer level.dat layouts
			n = int64(slices.Index(names, strings.ToLower(t.Text())))
		default:
			n = t.Int()
		}
		if n < 0 || n >= int64(len(names)) {
			return "", false
		}
		if named {
			return names[n], true
		}
		return strconv.FormatInt(n, 10), true
	}
	hardcore, difficulty := data.Get("hardcore"), data.Get("Difficulty")
	if ds := data.Get("difficulty_settings"); ds != nil {
		hardcore, difficulty = ds.Get("hardcore"), ds.Get("difficulty")
	}
	set := map[string]string{"hardcore": strconv.FormatBool(hardcore.Int() != 0)}
	if v, ok := value(difficulties, difficulty); ok {
		set["difficulty"] = v
	}
	if v, ok := value(gameModes, data.Get("GameType")); ok {
		set["gamemode"] = v
	}
	return set, data.Get("allowCommands").Int() != 0
}

// maxOps caps how much of ops.json is read.
const maxOps = 1 << 20

// addOp makes a player an operator of the server in dir, keeping every
// entry already there: ops.json, or ops.txt before 1.7.6.
func addOp(dir, mc, uuid, name string) error {
	if version.Compare(mc, "1.7.6") < 0 {
		return addOpTxt(filepath.Join(dir, "ops.txt"), name)
	}
	p := filepath.Join(dir, "ops.json")
	var ops []map[string]any
	data, err := fsx.ReadCapped(p, maxOps)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	case len(strings.TrimSpace(string(data))) > 0:
		if err := json.Unmarshal(data, &ops); err != nil {
			return err // never overwrite a file the server can't have written
		}
	}
	for _, o := range ops {
		if id, _ := o["uuid"].(string); strings.EqualFold(id, uuid) {
			return nil
		}
	}
	ops = append(ops, map[string]any{"uuid": uuid, "name": name, "level": 4, "bypassesPlayerLimit": false})
	out, err := json.MarshalIndent(ops, "", "  ")
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(p, append(out, '\n'), fsx.FilePerm)
}

func addOpTxt(p, name string) error {
	data, err := fsx.ReadCapped(p, maxOps)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), name) {
			return nil
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return fsx.AtomicWrite(p, []byte(text+name+"\n"), fsx.FilePerm)
}
