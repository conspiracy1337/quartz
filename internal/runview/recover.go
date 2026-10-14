package runview

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"

	"quartz/internal/core/fsx"
	"quartz/internal/core/procx"
)

// errUnreadable marks a saved view that can't be read back.
var errUnreadable = errors.New("runview: unreadable run view")

// load reads the view saved in a run's scratch folder.
func load(dir string) (state, error) {
	data, err := fsx.ReadCapped(filepath.Join(dir, stateFile), 1<<20)
	if err != nil {
		return state{}, err
	}
	var st state
	if json.Unmarshal(data, &st) != nil || st.Scratch != dir {
		return state{}, errUnreadable
	}
	return st, nil
}

// Recover merges back views the launcher couldn't close: it quit or crashed
// while their game ran. A view whose game still runs is left for next time.
func (a *Assembler) Recover(scratches []string) []Copy {
	var copies []Copy
	for _, dir := range scratches {
		st, err := load(dir)
		if errors.Is(err, fs.ErrNotExist) {
			_ = removeScratch(dir) // never got assembled
			continue
		}
		if err != nil {
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

// Adopt takes back the view of a game that outlived the launcher which
// built it: its folders count as used again, so a run started meanwhile
// gets a private view, until the adopted view is closed.
func (a *Assembler) Adopt(scratch string) (*View, error) {
	st, err := load(scratch)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	for _, p := range st.Leased {
		a.leases[p]++
	}
	a.mu.Unlock()
	return &View{a: a, st: st}, nil
}
