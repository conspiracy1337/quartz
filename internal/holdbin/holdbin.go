// Package holdbin moves files and folders aside inside a game folder so a
// removal can be undone for a while: content, screenshots and worlds all
// remove through it. Held items stay in the same folder tree, so moving them
// is a rename even when the game folder lives on another drive. Each hold
// keeps a small manifest beside its items, so Undo and the purge work after
// the launcher restarts.
package holdbin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"time"
)

// Dir is where held items wait, relative to the game folder.
const Dir = ".quartz/removed"

// Keep is how long a removal can be undone.
const Keep = 15 * time.Minute

const manifestName = "held.json"

// Errors.
var (
	ErrExpired   = errors.New("these items can't be brought back any more")
	ErrNameTaken = errors.New("a file with that name is already there")
)

var tokenRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ValidToken reports whether s looks like a hold token.
func ValidToken(s string) bool { return tokenRe.MatchString(s) }

// Item is one thing moved aside: where it was and where it waits, both
// slash-separated and relative to the game folder.
type Item struct {
	Rel      string `json:"rel"`
	Held     string `json:"held"`
	Optional bool   `json:"optional,omitempty"`
}

type manifest struct {
	At    int64  `json:"at"`
	Items []Item `json:"items"`
}

// Move names one path to hold. An optional move that fails is skipped (a
// .pw.toml beside a mod); a required one undoes the whole hold.
type Move struct {
	Rel      string
	Optional bool
}

// Hold moves paths aside and returns the token Undo takes. On a failed
// required move, everything moved so far goes back and the error is returned.
func Hold(root *os.Root, moves []Move, now time.Time) (string, []Item, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", nil, err
	}
	token := hex.EncodeToString(b[:])
	dir := path.Join(Dir, token)
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	var items []Item
	for n, m := range moves {
		it := Item{Rel: m.Rel, Held: path.Join(dir, strconv.Itoa(n)+"-"+path.Base(m.Rel)), Optional: m.Optional}
		if err := root.Rename(m.Rel, it.Held); err != nil {
			if m.Optional {
				continue
			}
			_ = putBack(root, items)
			_ = root.RemoveAll(dir)
			return "", nil, err
		}
		items = append(items, it)
	}
	data, _ := json.Marshal(manifest{At: now.Unix(), Items: items})
	if err := root.WriteFile(path.Join(dir, manifestName), data, 0o600); err != nil {
		_ = putBack(root, items)
		_ = root.RemoveAll(dir)
		return "", nil, err
	}
	return token, items, nil
}

// Restore puts a hold's items back and removes its folder. Items whose place
// is taken again stay held and are reported as ErrNameTaken.
func Restore(root *os.Root, token string) ([]Item, error) {
	if !ValidToken(token) {
		return nil, ErrExpired
	}
	dir := path.Join(Dir, token)
	m, err := read(root, dir)
	if err != nil {
		return nil, ErrExpired
	}
	err = putBack(root, m.Items)
	_ = root.RemoveAll(dir)
	return m.Items, err
}

func putBack(root *os.Root, items []Item) error {
	var errs []error
	for _, it := range items {
		if _, err := root.Lstat(it.Rel); err == nil {
			if !it.Optional {
				errs = append(errs, ErrNameTaken)
			}
			continue
		}
		_ = root.MkdirAll(path.Dir(it.Rel), 0o700)
		if err := root.Rename(it.Held, it.Rel); err != nil && !it.Optional {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Purge deletes holds older than Keep. drop, if set, is called for each held
// item first (content unlinks store files through the store there).
func Purge(root *os.Root, now time.Time, drop func(token string, it Item)) {
	d, err := root.Open(Dir)
	if err != nil {
		return
	}
	names, _ := d.Readdirnames(-1)
	_ = d.Close()
	cutoff := now.Add(-Keep).Unix()
	for _, token := range names {
		if !ValidToken(token) {
			continue
		}
		dir := path.Join(Dir, token)
		m, err := read(root, dir)
		if err != nil {
			// No readable manifest: fall back to the folder's age.
			if fi, serr := root.Stat(dir); serr != nil || fi.ModTime().Unix() > cutoff {
				continue
			}
		} else if m.At > cutoff {
			continue
		}
		if drop != nil && m != nil {
			for _, it := range m.Items {
				drop(token, it)
			}
		}
		_ = root.RemoveAll(dir)
	}
}

func read(root *os.Root, dir string) (*manifest, error) {
	f, err := root.Open(path.Join(dir, manifestName))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
