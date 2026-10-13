package discord

import "strconv"

// candidates are Discord's named pipes.
func candidates() []string {
	out := make([]string, 10)
	for i := range out {
		out[i] = `\\.\pipe\discord-ipc-` + strconv.Itoa(i)
	}
	return out
}
