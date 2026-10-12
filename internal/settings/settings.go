// Package settings owns the launcher's global settings document: its schema,
// defaults, validation and persistence.
package settings

import (
	"regexp"
	"strings"
	"unicode"
)

// SchemaVersion is bumped whenever a migration is needed.
const SchemaVersion = 1

// Settings is the whole global settings document. Every field has a default and
// every value is validated by Normalize, so a hand-edited or older file can never
// put the launcher into an invalid state.
type Settings struct {
	Version      int          `json:"version"`
	General      General      `json:"general"`
	Downloads    Downloads    `json:"downloads"`
	Integrations Integrations `json:"integrations"`
	Appearance   Appearance   `json:"appearance"`
	Java         Java         `json:"java"`
	Updates      Updates      `json:"updates"`
}

type General struct {
	Language          string `json:"language"`    // "system" or a locale tag
	OnGameStart       string `json:"onGameStart"` // keep-open | hide | close
	StartPage         string `json:"startPage"`   // home | library | last-instance
	StartWithComputer bool   `json:"startWithComputer"`
}

type Downloads struct {
	Parallel       int         `json:"parallel"` // 1–16
	WatchDownloads bool        `json:"watchDownloads"`
	Proxy          string      `json:"proxy"` // system | none | custom
	ProxyCustom    ProxyCustom `json:"proxyCustom"`
}

// ProxyCustom is used when Proxy is "custom". The password is never part of
// the settings: it lives in the OS keychain (see api.Settings.SetProxyPassword).
type ProxyCustom struct {
	Type string `json:"type"` // http | https | socks5
	Host string `json:"host"` // name or IP, no scheme
	Port int    `json:"port"` // 1–65535
	User string `json:"user"`
}

// Usable reports whether the custom proxy is complete enough to use.
func (p ProxyCustom) Usable() bool { return p.Host != "" }

type Integrations struct {
	DiscordStatus bool `json:"discordStatus"`
	HandleLinks   bool `json:"handleLinks"`
}

type Appearance struct {
	Theme        string    `json:"theme"`        // monolith | graphite | oled | light | system
	Accent       string    `json:"accent"`       // #rrggbb
	Density      string    `json:"density"`      // comfortable | compact
	TextSize     string    `json:"textSize"`     // small | default | large
	ReduceMotion string    `json:"reduceMotion"` // system | on | off
	CustomCSS    CustomCSS `json:"customCss"`
}

type CustomCSS struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

type Java struct {
	AutoPick       bool   `json:"autoPick"`
	DownloadVendor string `json:"downloadVendor"` // temurin | zulu | mojang
	MemoryMinMB    int    `json:"memoryMinMb"`
	MemoryMaxMB    int    `json:"memoryMaxMb"`
	JVMArgs        string `json:"jvmArgs"`
	WarnLowMemory  bool   `json:"warnLowMemory"`
	SuggestMemory  bool   `json:"suggestMemory"`
}

type Updates struct {
	AutoCheck  bool   `json:"autoCheck"`  // check for launcher updates in the background
	ModChannel string `json:"modChannel"` // release | beta | alpha
}

// Defaults returns the settings a fresh install starts with.
func Defaults() Settings {
	return Settings{
		Version: SchemaVersion,
		General: General{Language: "system", OnGameStart: "hide", StartPage: "home"},
		Downloads: Downloads{
			Parallel: 8, WatchDownloads: true, Proxy: "system",
			ProxyCustom: ProxyCustom{Type: "http", Port: 8080},
		},
		Integrations: Integrations{DiscordStatus: true, HandleLinks: true},
		Appearance: Appearance{
			Theme: "monolith", Accent: "#3ecf8e", Density: "comfortable",
			TextSize: "default", ReduceMotion: "system",
		},
		Java: Java{
			AutoPick: true, DownloadVendor: "temurin", MemoryMinMB: 1024, MemoryMaxMB: 4096,
			JVMArgs: "-XX:+UseG1GC -XX:+ParallelRefProcEnabled", WarnLowMemory: true, SuggestMemory: true,
		},
		Updates: Updates{AutoCheck: true, ModChannel: "release"},
	}
}

var (
	hexColor  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	proxyHost = regexp.MustCompile(`^(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9.-]{1,253})$`)
	localeTag = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
)

const (
	maxJVMArgs  = 4096
	minMemoryMB = 256
	maxMemoryMB = 1 << 20
	maxPathLen  = 4096
	maxUserLen  = 256
)

// Normalize replaces every invalid field with its default, field by field, so one
// bad value never discards the rest of the document.
func (s Settings) Normalize() Settings {
	d := Defaults()
	s.Version = SchemaVersion

	if s.General.Language != "system" && !localeTag.MatchString(s.General.Language) {
		s.General.Language = d.General.Language
	}
	s.General.OnGameStart = oneOf(s.General.OnGameStart, d.General.OnGameStart, "keep-open", "hide", "close")
	s.General.StartPage = oneOf(s.General.StartPage, d.General.StartPage, "home", "library", "last-instance")

	s.Downloads.Parallel = clamp(s.Downloads.Parallel, 1, 16, d.Downloads.Parallel)
	s.Downloads.Proxy = oneOf(s.Downloads.Proxy, d.Downloads.Proxy, "system", "none", "custom")
	s.Downloads.ProxyCustom = s.Downloads.ProxyCustom.normalize(d.Downloads.ProxyCustom)

	a := &s.Appearance
	a.Theme = oneOf(a.Theme, d.Appearance.Theme, "monolith", "graphite", "oled", "light", "system")
	if !hexColor.MatchString(a.Accent) {
		a.Accent = d.Appearance.Accent
	}
	a.Density = oneOf(a.Density, d.Appearance.Density, "comfortable", "compact")
	a.TextSize = oneOf(a.TextSize, d.Appearance.TextSize, "small", "default", "large")
	a.ReduceMotion = oneOf(a.ReduceMotion, d.Appearance.ReduceMotion, "system", "on", "off")
	if len(a.CustomCSS.Path) > maxPathLen {
		a.CustomCSS = CustomCSS{}
	}

	j := &s.Java
	j.DownloadVendor = oneOf(j.DownloadVendor, d.Java.DownloadVendor, "temurin", "zulu", "mojang")
	j.MemoryMinMB = clamp(j.MemoryMinMB, minMemoryMB, maxMemoryMB, d.Java.MemoryMinMB)
	j.MemoryMaxMB = clamp(j.MemoryMaxMB, minMemoryMB, maxMemoryMB, d.Java.MemoryMaxMB)
	if j.MemoryMaxMB < j.MemoryMinMB {
		j.MemoryMaxMB = j.MemoryMinMB
	}
	if len(j.JVMArgs) > maxJVMArgs {
		j.JVMArgs = d.Java.JVMArgs
	}
	s.Updates.ModChannel = oneOf(s.Updates.ModChannel, d.Updates.ModChannel, "release", "beta", "alpha")
	return s
}

// normalize keeps a half-typed proxy (empty host) but drops anything malformed:
// a host with a scheme, path or spaces, an out-of-range port, a control
// character in the user name.
func (p ProxyCustom) normalize(def ProxyCustom) ProxyCustom {
	p.Type = oneOf(p.Type, def.Type, "http", "https", "socks5")
	if p.Host != "" && !proxyHost.MatchString(p.Host) {
		p.Host = ""
	}
	p.Port = clamp(p.Port, 1, 65535, def.Port)
	if len(p.User) > maxUserLen || strings.ContainsFunc(p.User, unicode.IsControl) {
		p.User = ""
	}
	return p
}

func oneOf(v, def string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return def
}

func clamp(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	return min(max(v, lo), hi)
}

// Resolve returns override when it is set and global otherwise. Instance settings
// use pointer fields (nil = inherit) and resolve each one against the global value,
// field by field, as in Prism's layered settings.
func Resolve[T any](override *T, global T) T {
	if override != nil {
		return *override
	}
	return global
}
