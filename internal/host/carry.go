package host

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"quartz/internal/core/nbt"
)

// A singleplayer world keeps its player in level.dat (Data.Player); a
// dedicated server keeps every player in playerdata/<uuid>.dat. Carrying the
// one into the other lets you host a world you played alone and keep your
// inventory, and play it alone again afterwards where the server left off.

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// dashed formats a 32-digit UUID the way playerdata files are named.
func dashed(id string) string {
	if len(id) == 32 {
		id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	}
	return id
}

func playerFile(world, uuid string) (string, error) {
	if !uuidRe.MatchString(uuid) {
		return "", errors.New("host: not a player id")
	}
	return filepath.Join(world, "playerdata", uuid+".dat"), nil
}

// carryIn copies the singleplayer player into the server's player file,
// unless the server's file is newer (the player was on the server since).
// Worlds older than 1.9 (no DataVersion) are left alone.
func carryIn(world, uuid string) error {
	pf, err := playerFile(world, uuid)
	if err != nil {
		return err
	}
	lvl := filepath.Join(world, "level.dat")
	f, err := nbt.ReadFile(lvl)
	if err != nil {
		return ignoreMissing(err)
	}
	data := f.Root.Get("Data")
	if data == nil || data.Get("DataVersion") == nil {
		return nil
	}
	player := data.Get("Player")
	if player == nil || player.Kind != nbt.Compound {
		return nil
	}
	if newer(pf, lvl) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(pf), 0o700); err != nil {
		return err
	}
	return nbt.WriteFile(pf, &nbt.File{Root: *player, Gzip: true})
}

// carryOut writes the server's player file back into level.dat when the
// server saved it after since.
func carryOut(world, uuid string, since time.Time) error {
	pf, err := playerFile(world, uuid)
	if err != nil {
		return err
	}
	info, err := os.Stat(pf)
	if err != nil || !info.ModTime().After(since) {
		return ignoreMissing(err)
	}
	p, err := nbt.ReadFile(pf)
	if err != nil || p.Root.Kind != nbt.Compound {
		return err
	}
	lvl := filepath.Join(world, "level.dat")
	f, err := nbt.ReadFile(lvl)
	if err != nil {
		return err
	}
	data := f.Root.Get("Data")
	if data == nil || data.Kind != nbt.Compound || data.Get("DataVersion") == nil {
		return nil
	}
	data.Comp.Set("Player", p.Root)
	return nbt.WriteFile(lvl, f)
}

// newer reports whether a was modified after b (a missing a never is).
func newer(a, b string) bool {
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	return err != nil || ia.ModTime().After(ib.ModTime())
}

func ignoreMissing(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
