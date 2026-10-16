package servers

import (
	"context"
	"regexp"
	"strings"

	"quartz/internal/core/slp"
	"quartz/internal/core/version"
)

// Status is what a ping found out.
type Status struct {
	Online  bool   `json:"online"`
	Version string `json:"version,omitempty"` // "1.21.4", "1.8 to 1.21"
	Players int    `json:"players"`
	Max     int    `json:"max,omitempty"`
	PingMs  int    `json:"pingMs,omitempty"`
	Icon    string `json:"icon,omitempty"` // the favicon on the image route
	// Fit is how the server suits the instance's Minecraft version (see fit),
	// and FitVersion the version to show beside it.
	Fit        string `json:"fit,omitempty"`
	FitVersion string `json:"fitVersion,omitempty"`
}

// How a server suits a Minecraft version.
const (
	FitOK    = "ok"    // it takes this version
	FitMinor = "minor" // it takes another version of the same major one ("1.21.1" for "1.21.4")
	FitMajor = "major" // it takes only another major version
)

// status pings address for an instance on minecraft, speaking its protocol
// (or asking the server which it wants when the protocol is unknown).
func (s *Service) status(ctx context.Context, address, minecraft string, force bool) Status {
	a, err := slp.Parse(address)
	if err != nil {
		return Status{}
	}
	protocol := s.d.Protocol(minecraft)
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
	match := 0
	if protocol > 0 && r.Status.Protocol > 0 {
		match = -1
		if r.Status.Protocol == protocol {
			match = 1
		}
	}
	st.Fit, st.FitVersion = fit(minecraft, r.Status.Version, match)
	return st
}

// fit says how a server whose version name is label suits minecraft, and
// which version to show: minecraft itself when the server takes it, else the
// newest version the server takes. match is 1 when the ping's protocols
// agree, -1 when they differ and 0 when either is unknown; then the
// server's version range decides ("1.8 to 1.21" takes every 1.21.x). A
// server whose name holds no version gets no fit.
func fit(minecraft, label string, match int) (string, string) {
	found := versionRe.FindAllString(label, -1)
	if match == 1 {
		return FitOK, minecraft
	}
	if len(found) == 0 {
		return "", ""
	}
	lo, hi := found[0], found[len(found)-1]
	if match == 0 && version.Compare(lo, minecraft) <= 0 && (version.Compare(minecraft, hi) <= 0 || hi == major(hi) && major(minecraft) == hi) {
		return FitOK, minecraft
	}
	if major(hi) == major(minecraft) {
		return FitMinor, hi
	}
	return FitMajor, hi
}

// major is a version without its patch number: "1.21" of "1.21.4" and of
// "1.21", "26.1" of "26.1.2". A snapshot ("25w14a") is its own.
func major(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
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
