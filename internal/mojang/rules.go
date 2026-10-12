package mojang

import (
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Env is the platform and feature set rules are evaluated against.
type Env struct {
	OS        string          // linux | osx | windows
	Arch      string          // x86_64 | arm64 | x86
	OSVersion string          // e.g. "10.0.19045" on Windows
	Features  map[string]bool // has_custom_resolution, is_quick_play_singleplayer, …
}

// CurrentEnv describes the running machine (features are set by the caller).
func CurrentEnv(osVersion string) Env {
	return Env{OS: OSName(runtime.GOOS), Arch: ArchName(runtime.GOARCH), OSVersion: osVersion, Features: map[string]bool{}}
}

// OSName maps a GOOS to Mojang's name.
func OSName(goos string) string {
	switch goos {
	case "darwin":
		return "osx"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// ArchName maps a GOARCH to Mojang's name ("x86" means 32-bit).
func ArchName(goarch string) string {
	switch goarch {
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return "x86_64"
	}
}

// Rule allows or disallows something on matching platforms or features.
type Rule struct {
	Action   string          `json:"action"`
	OS       *OSRule         `json:"os,omitempty"`
	Features map[string]bool `json:"features,omitempty"`
}

// OSRule matches a platform. Version is a regular expression (older JSONs);
// VersionRange bounds the OS version (newer JSONs; min inclusive, max exclusive).
type OSRule struct {
	Name         string        `json:"name,omitempty"`
	Arch         string        `json:"arch,omitempty"`
	Version      string        `json:"version,omitempty"`
	VersionRange *VersionRange `json:"versionRange,omitempty"`
}

// VersionRange bounds a dotted OS version.
type VersionRange struct {
	Min string `json:"min,omitempty"`
	Max string `json:"max,omitempty"`
}

// Allowed evaluates rules: with no rules everything is allowed; otherwise the
// default is deny and the last matching rule decides.
func Allowed(rules []Rule, env Env) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, r := range rules {
		if r.matches(env) {
			allowed = r.Action == "allow"
		}
	}
	return allowed
}

func (r Rule) matches(env Env) bool {
	if o := r.OS; o != nil {
		if o.Name != "" && o.Name != env.OS {
			return false
		}
		if o.Arch != "" && o.Arch != env.Arch {
			return false
		}
		if o.Version != "" {
			// RE2 cannot be driven into catastrophic backtracking by hostile metadata.
			re, err := regexp.Compile(o.Version)
			if err != nil || !re.MatchString(env.OSVersion) {
				return false
			}
		}
		if vr := o.VersionRange; vr != nil {
			if vr.Min != "" && compareVersions(env.OSVersion, vr.Min) < 0 {
				return false
			}
			if vr.Max != "" && compareVersions(env.OSVersion, vr.Max) >= 0 {
				return false
			}
		}
	}
	for k, want := range r.Features {
		if env.Features[k] != want {
			return false
		}
	}
	return true
}

// compareVersions compares dotted numeric versions ("10.0.17134").
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
