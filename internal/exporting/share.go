package exporting

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"quartz/internal/brand"
	"quartz/internal/core/netx"
)

// shareURL is where mod lists are stored for a code (the launcher's web API).
const shareURL = brand.WebAPI + "modlists"

// ShareTTL is how long a code stays valid.
const ShareTTL = 7 * 24 * time.Hour

// Code is a shared mod list's code and when it stops working.
type Code struct {
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expiresAt"` // unix ms
}

// ErrShare is returned when no code could be made.
var ErrShare = errors.New("exporting: couldn't create a code for this mod list")

var codeRe = regexp.MustCompile(`^[A-Z0-9]{5}$`)

// Share stores id's mod list on the launcher's web API and returns its code.
// Only the list goes out: names, versions and download addresses, never
// paths, accounts or the instance's notes.
func (s *Service) Share(ctx context.Context, id string) (Code, error) {
	l, err := s.modList(ctx, id)
	if err != nil {
		return Code{}, err
	}
	if len(l.list.Mods) == 0 {
		return Code{}, ErrNoMods
	}
	if s.d.HTTP == nil {
		return Code{}, ErrShare
	}
	var out struct {
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	ctx = netx.WithBodyCap(ctx, 4<<10) // a code and a date
	if err := s.d.HTTP.JSON(ctx, http.MethodPost, orElse(s.d.shareURL, shareURL), nil, l.list, &out); err != nil {
		return Code{}, fmt.Errorf("%w (%w)", ErrShare, err)
	}
	now := s.now()
	if !codeRe.MatchString(out.Code) || !out.ExpiresAt.After(now) || out.ExpiresAt.After(now.Add(ShareTTL+24*time.Hour)) {
		return Code{}, ErrShare
	}
	return Code{Code: out.Code, ExpiresAt: out.ExpiresAt.UnixMilli()}, nil
}

func (s *Service) now() time.Time {
	if s.d.Now != nil {
		return s.d.Now()
	}
	return time.Now()
}
