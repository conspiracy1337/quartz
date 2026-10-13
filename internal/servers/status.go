package servers

import (
	"context"
	"regexp"
	"strings"

	"quartz/internal/core/slp"
)

// Status is what a ping found out.
type Status struct {
	Online  bool   `json:"online"`
	Version string `json:"version,omitempty"` // "1.21.4", "1.8 to 1.21"
	Players int    `json:"players"`
	Max     int    `json:"max,omitempty"`
	PingMs  int    `json:"pingMs,omitempty"`
	Icon    string `json:"icon,omitempty"` // the favicon on the image route
	// Needs names the version the server wants when it can't take this
	// instance's ("Needs 1.20.1"); empty when they match or it's unknown.
	Needs string `json:"needs,omitempty"`
}

// status pings address speaking protocol (0 asks the server which it wants).
func (s *Service) status(ctx context.Context, address string, protocol int, force bool) Status {
	a, err := slp.Parse(address)
	if err != nil {
		return Status{}
	}
	proto := protocol
	if proto == 0 {
		proto = -1
	}
	r := s.d.Pinger.Get(ctx, a, proto, force)
	if r.Err != nil || r.Status == nil {
		return Status{}
	}
	st := Status{
		Online:  true,
		Version: VersionLabel(r.Status.Version),
		Players: r.Status.Online,
		Max:     r.Status.Max,
		PingMs:  max(1, int(r.Status.Latency.Milliseconds())),
	}
	if s.d.Favicons != nil && len(r.Status.Favicon) > 0 {
		st.Icon = s.d.Favicons.Put(r.Status.Favicon)
	}
	if protocol > 0 && r.Status.Protocol > 0 && r.Status.Protocol != protocol {
		st.Needs = st.Version
	}
	return st
}

var versionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

// VersionLabel shortens what a server calls its version: "Paper 1.21.4" is
// "1.21.4", "Requires MC 1.8 / 1.21" is "1.8 to 1.21". Names without a
// version number are kept as they are.
func VersionLabel(name string) string {
	found := versionRe.FindAllString(name, -1)
	switch {
	case len(found) == 0:
		return clip(strings.TrimSpace(name), 24)
	case len(found) == 1 || found[0] == found[len(found)-1]:
		return found[0]
	}
	return found[0] + " to " + found[len(found)-1]
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
