package runview

import (
	"encoding/json"
	"os"
	"path/filepath"

	"quartz/internal/core/fsx"
	"quartz/internal/core/procx"
)

// Recover merges back views the launcher couldn't close: it quit or crashed
// while their game ran. A view whose game still runs is left for next time.
func (a *Assembler) Recover(scratches []string) []Copy {
	var copies []Copy
	for _, dir := range scratches {
		data, err := fsx.ReadCapped(filepath.Join(dir, stateFile), 1<<20)
		if os.IsNotExist(err) {
			_ = removeScratch(dir) // never got assembled
			continue
		}
		var st state
		if err != nil || json.Unmarshal(data, &st) != nil || st.Scratch != dir {
			a.c.Log.Warn("unreadable run view left as is", "dir", dir)
			continue
		}
		if procx.Alive(st.PID) {
			continue
		}
		c, err := st.close(a.c.Retitle)
		if err != nil {
			a.c.Log.Warn("merging back a run view failed", "dir", dir, "err", err)
		}
		copies = append(copies, c...)
	}
	return copies
}
