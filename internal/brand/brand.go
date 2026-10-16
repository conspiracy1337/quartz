// Package brand is the single place that names the product. Nothing else in the
// codebase may hard-code the launcher's name, folder, scheme or IDs.
package brand

import (
	"os"
	"regexp"
	"strings"
)

const (
	// Name is the full product name shown in the UI and window title.
	Name = "Quartz Client"
	// ShortName is used where space is tight (sidebar brand, tray).
	ShortName = "Quartz"
	// Slug names the binary and the link scheme.
	Slug = "quartz"
	// Scheme is the launcher's own URL scheme (quartz://…).
	Scheme = Slug
	// DirName is the launcher's folder inside the OS data and config
	// directories, and inside a chosen folder that holds other things.
	DirName = Slug
	// EnvDir overrides the data folder location.
	EnvDir = "QUARTZ_DIR"
	// PortableMarker, placed next to the executable, switches on portable mode.
	PortableMarker = "portable.txt"
	// AppID identifies the app for the single-instance lock and OS integration.
	AppID = "dev.quartzclient.launcher"
)

// EnvMSAClientID overrides MSAClientID, for development builds.
const EnvMSAClientID = "QUARTZ_MSA_CLIENT_ID"

// EnvCurseForgeKey overrides CurseForgeAPIKey.
const EnvCurseForgeKey = "QUARTZ_CF_API_KEY"

// Version is set at build time with -ldflags "-X quartz/internal/brand.Version=…".
var Version = "0.0.0-dev"

// MSAClientID is the Azure application (client) ID used for Microsoft sign-in,
// set at build time with -ldflags "-X quartz/internal/brand.MSAClientID=…".
// It is public, not a secret, but it must be approved by Mojang for the
// Minecraft services API, so it is never hard-coded here.
var MSAClientID = ""

var guid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidClientID reports whether id is a well-formed Microsoft client ID.
func ValidClientID(id string) bool { return guid.MatchString(id) }

// ClientID returns the Microsoft client ID ($QUARTZ_MSA_CLIENT_ID first, then
// the build value), or "" when neither is a well-formed GUID. An empty result
// means Microsoft sign-in is not set up in this build.
func ClientID() string {
	for _, id := range []string{os.Getenv(EnvMSAClientID), MSAClientID} {
		if ValidClientID(id) {
			return strings.ToLower(id)
		}
	}
	return ""
}

// CurseForgeAPIKey is the key for CurseForge's API (console.curseforge.com),
// set at build time with -ldflags "-X quartz/internal/brand.CurseForgeAPIKey=…".
// It is never hard-coded, and it is only ever sent to CurseForge's API host.
var CurseForgeAPIKey = ""

var apiKey = regexp.MustCompile(`^[\x21-\x7e]{20,128}$`)

// ValidCurseForgeKey reports whether k is a well-formed CurseForge API key.
func ValidCurseForgeKey(k string) bool { return apiKey.MatchString(k) }

// CurseForgeKey returns the CurseForge API key ($QUARTZ_CF_API_KEY first, then
// the build value), or "" when neither is well-formed. Without a key,
// CurseForge lookups are skipped.
func CurseForgeKey() string {
	for _, k := range []string{os.Getenv(EnvCurseForgeKey), CurseForgeAPIKey} {
		if ValidCurseForgeKey(k) {
			return k
		}
	}
	return ""
}

// EnvDiscordClientID overrides DiscordClientID.
const EnvDiscordClientID = "QUARTZ_DISCORD_CLIENT_ID"

// DiscordClientID is the Discord application that Rich Presence shows ("Playing
// Quartz Client"), set at build time with
// -ldflags "-X quartz/internal/brand.DiscordClientID=…". It is public.
var DiscordClientID = ""

var snowflake = regexp.MustCompile(`^[0-9]{17,20}$`)

// ValidDiscordID reports whether id is a well-formed Discord application ID.
func ValidDiscordID(id string) bool { return snowflake.MatchString(id) }

// DiscordID returns the Discord application ID ($QUARTZ_DISCORD_CLIENT_ID
// first, then the build value), or "" when neither is well-formed: Rich
// Presence is then not set up in this build.
func DiscordID() string {
	for _, id := range []string{os.Getenv(EnvDiscordClientID), DiscordClientID} {
		if ValidDiscordID(id) {
			return id
		}
	}
	return ""
}

// WebAPI is the launcher's own web API; shared mod lists are stored there.
const WebAPI = "https://quartz.conspiracy.moe/api/"

// UpdateRepo is the GitHub repository whose releases update the launcher.
const UpdateRepo = "conspiracy1337/quartz"

// UpdatePublicKey is the minisign public key releases are signed with, set
// at build time with -ldflags "-X quartz/internal/brand.UpdatePublicKey=…"
// (the base64 line of the .pub file). Without it the launcher never
// updates itself.
var UpdatePublicKey = ""

// EnvUpdateBase points update checks at another release server (testing a
// release before it is published). Downloads are still verified against
// UpdatePublicKey.
const EnvUpdateBase = "QUARTZ_UPDATE_BASE"
