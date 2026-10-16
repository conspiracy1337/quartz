package sysinfo

import (
	"os/exec"
	"runtime"
)

// The Linux programs the launcher can put in front of the game.
const (
	GameModeBin = "gamemoderun"
	MangoHudBin = "mangohud"
)

// Tools says which optional game helpers are installed. Each lookup is a few
// stats along PATH, so it is checked fresh: installing one shows up at once.
type Tools struct {
	GameMode bool `json:"gameMode"`
	MangoHud bool `json:"mangoHud"`
}

// FindTools looks the helpers up on PATH (Linux only; elsewhere none exist).
func FindTools() Tools {
	if runtime.GOOS != "linux" {
		return Tools{}
	}
	return Tools{GameMode: onPath(GameModeBin), MangoHud: onPath(MangoHudBin)}
}

func onPath(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
