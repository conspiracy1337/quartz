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

// Opened records that the instance's page was opened. It writes at most once
// a minute; changed is false when nothing was written.
func (s *Store) Opened(id string, now time.Time) (changed bool, err error) {
	in, err := s.Get(id)
	if err != nil || in.LastOpened != nil && now.Sub(*in.LastOpened) < time.Minute {
		return false, err
	}
	_, err = s.Update(id, func(in *Instance) error { t := now.UTC(); in.LastOpened = &t; return nil })
	return err == nil, err
}

// Look is how an instance shows: its tile colour and letter, its picture
// and its banner ("" for none). Colour, picture and banner are independent:
// a new colour keeps the pictures, and shows behind a transparent one.
type Look struct {
	Art    Art    `json:"art"`
	Icon   string `json:"icon"`
	Banner string `json:"banner"`
}

// validLook checks a look's letter and picture names; a colour that isn't
// one is repaired by normalize.
func validLook(art Art, icon, banner string) bool {
	r := []rune(strings.TrimSpace(art.Initial))
	return len(r) <= 2 && !slices.ContainsFunc(r, unicode.IsControl) &&
		(icon == "" || iconRe.MatchString(icon)) && (banner == "" || iconRe.MatchString(banner))
}

// SetLook sets an instance's colour, letter, picture and banner.
func (s *Store) SetLook(id string, l Look) (Instance, error) {
	if !hexColor.MatchString(l.Art.Background) || !validLook(l.Art, l.Icon, l.Banner) {
		return Instance{}, fmt.Errorf("%w: look", ErrInvalidSettings)
	}
	l.Art.Initial = strings.TrimSpace(l.Art.Initial)
	return s.Update(id, func(in *Instance) error {
		if l.Art.Initial == "" {
			l.Art.Initial = strings.ToUpper(string([]rune(in.Name)[:1]))
		}
		in.Art, in.Icon, in.Banner = l.Art, l.Icon, l.Banner
		return nil
	})
}

// SetIcon uses a picture from the icon library ("" goes back to the tile).
func (s *Store) SetIcon(id, hash string) (Instance, error) {
	if !validLook(Art{}, hash, "") {
		return Instance{}, fmt.Errorf("%w: icon", ErrInvalidSettings)
	}
	return s.Update(id, func(in *Instance) error { in.Icon = hash; return nil })
}

// SetBanner uses a picture from the banner library ("" goes back to the colour).
func (s *Store) SetBanner(id, hash string) (Instance, error) {
	if !validLook(Art{}, "", hash) {
		return Instance{}, fmt.Errorf("%w: banner", ErrInvalidSettings)
	}
	return s.Update(id, func(in *Instance) error { in.Banner = hash; return nil })
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
