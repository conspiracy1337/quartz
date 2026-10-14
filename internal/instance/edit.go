package instance

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"quartz/internal/sandbox/rules"
)

// Rename changes the display name only: the id and folder never change.
func (s *Store) Rename(id, name string) (Instance, error) {
	n, err := cleanName(name)
	if err != nil {
		return Instance{}, err
	}
	return s.Update(id, func(in *Instance) error { in.Name = n; return nil })
}

// SetArt sets the tile colour and letter and drops any picture.
func (s *Store) SetArt(id string, art Art) (Instance, error) {
	if !hexColor.MatchString(art.Background) {
		return Instance{}, fmt.Errorf("%w: colour", ErrInvalidSettings)
	}
	if r := []rune(strings.TrimSpace(art.Initial)); len(r) > 2 || slices.ContainsFunc(r, unicode.IsControl) {
		return Instance{}, fmt.Errorf("%w: letter", ErrInvalidSettings)
	}
	art.Initial = strings.TrimSpace(art.Initial)
	return s.Update(id, func(in *Instance) error {
		if art.Initial == "" {
			art.Initial = strings.ToUpper(string([]rune(in.Name)[:1]))
		}
		in.Art, in.Icon = art, ""
		return nil
	})
}

// SetIcon uses a picture from the icon library ("" goes back to the tile).
func (s *Store) SetIcon(id, hash string) (Instance, error) {
	if hash != "" && !iconRe.MatchString(hash) {
		return Instance{}, fmt.Errorf("%w: icon", ErrInvalidSettings)
	}
	return s.Update(id, func(in *Instance) error { in.Icon = hash; return nil })
}

// SetNotes stores free-form notes.
func (s *Store) SetNotes(id, notes string) (Instance, error) {
	if len([]rune(notes)) > maxNotes || strings.ContainsRune(notes, 0) {
		return Instance{}, fmt.Errorf("%w: notes are limited to %d characters", ErrInvalidSettings, maxNotes)
	}
	return s.Update(id, func(in *Instance) error { in.Notes = notes; return nil })
}

// SetTags replaces the tags (trimmed, de-duplicated, at most 12 of 24 characters).
func (s *Store) SetTags(id string, tags []string) (Instance, error) {
	if len(tags) > maxTags*2 {
		return Instance{}, fmt.Errorf("%w: too many tags", ErrInvalidSettings)
	}
	clean := cleanTags(tags)
	return s.Update(id, func(in *Instance) error { in.Tags = clean; return nil })
}

// SetVersionOptions stores the two Version-tab switches.
func (s *Store) SetVersionOptions(id string, keepLoaderUpdated, followLatest bool) (Instance, error) {
	return s.Update(id, func(in *Instance) error {
		in.KeepLoaderUpdated, in.FollowLatest = keepLoaderUpdated, followLatest && in.ClientJar == nil
		return nil
	})
}

// SetDiscord turns the instance's Discord status on or off.
func (s *Store) SetDiscord(id string, on bool) (Instance, error) {
	return s.Update(id, func(in *Instance) error {
		in.HideDiscord = !on
		return nil
	})
}

// TotalPlaytime is the minutes played across every instance.
func (s *Store) TotalPlaytime() int {
	n := 0
	for _, in := range s.List() {
		n += in.PlaytimeMin
	}
	return n
}

// EditSandbox changes an instance's sandbox choices; edit validates them
// (package sandbox) and the result is normalized.
func (s *Store) EditSandbox(id string, edit func(*rules.Settings) error) (Instance, error) {
	return s.Update(id, func(in *Instance) error {
		next := in.Sandbox
		if err := edit(&next); err != nil {
			return err
		}
		in.Sandbox = next.Normalize()
		return nil
	})
}

// AddPlaytime counts a session's length, carrying seconds between sessions.
func (in *Instance) AddPlaytime(d time.Duration) {
	total := in.PlaytimeSec + int(max(d, 0)/time.Second)
	in.PlaytimeMin += total / 60
	in.PlaytimeSec = total % 60
}
