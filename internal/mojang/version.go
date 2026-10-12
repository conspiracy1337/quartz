// Package mojang parses Mojang's launcher metadata: the version manifest,
// version JSONs (with inheritsFrom merging for loader profiles), rules,
// arguments, libraries and natives, asset indexes and the Java runtime index.
// It does no I/O. Every value comes from the network, so anything that later
// becomes a path or a URL is validated here.
package mojang

import (
	"encoding/json"
	"time"
)

// Version is one version JSON (vanilla or a loader profile).
type Version struct {
	ID                 string       `json:"id"`
	Type               string       `json:"type"`
	InheritsFrom       string       `json:"inheritsFrom,omitempty"`
	MainClass          string       `json:"mainClass"`
	MinecraftArguments string       `json:"minecraftArguments,omitempty"`
	Arguments          *Arguments   `json:"arguments,omitempty"`
	AssetIndex         *AssetRef    `json:"assetIndex,omitempty"`
	Assets             string       `json:"assets,omitempty"`
	Downloads          Downloads    `json:"downloads,omitempty"`
	JavaVersion        *JavaVersion `json:"javaVersion,omitempty"`
	Libraries          []Library    `json:"libraries"`
	Logging            *Logging     `json:"logging,omitempty"`
	ReleaseTime        time.Time    `json:"releaseTime"`
}

// Downloads names the version's own files ("client", "server", …).
type Downloads map[string]Artifact

// Artifact is one downloadable file. Path is only set for libraries.
type Artifact struct {
	Path string `json:"path,omitempty"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// AssetRef points to the asset index.
type AssetRef struct {
	ID        string `json:"id"`
	SHA1      string `json:"sha1"`
	Size      int64  `json:"size"`
	TotalSize int64  `json:"totalSize"`
	URL       string `json:"url"`
}

// JavaVersion is the runtime a version needs.
type JavaVersion struct {
	Component    string `json:"component"`
	MajorVersion int    `json:"majorVersion"`
}

// Logging configures log4j for the client.
type Logging struct {
	Client *LoggingClient `json:"client,omitempty"`
}

// LoggingClient is the client's log4j configuration file and its JVM argument.
type LoggingClient struct {
	Argument string     `json:"argument"`
	File     LoggedFile `json:"file"`
	Type     string     `json:"type"`
}

// LoggedFile is the log4j XML file.
type LoggedFile struct {
	ID   string `json:"id"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// JavaMajor returns the Java major version the game needs (8 when unstated).
func (v *Version) JavaMajor() int {
	if v.JavaVersion != nil && v.JavaVersion.MajorVersion > 0 {
		return v.JavaVersion.MajorVersion
	}
	return 8
}

// JavaComponent returns Mojang's runtime component name ("jre-legacy" when unstated).
func (v *Version) JavaComponent() string {
	if v.JavaVersion != nil && v.JavaVersion.Component != "" {
		return v.JavaVersion.Component
	}
	return "jre-legacy"
}

// AssetsID is the asset index id the game expects.
func (v *Version) AssetsID() string {
	if v.AssetIndex != nil && v.AssetIndex.ID != "" {
		return v.AssetIndex.ID
	}
	if v.Assets != "" {
		return v.Assets
	}
	return "legacy"
}

// Parse decodes a version JSON.
func Parse(data []byte) (*Version, error) {
	var v Version
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// Merge applies a child profile (e.g. Fabric, which sets inheritsFrom) on top
// of its parent, the way the official launcher does: the child's libraries
// come first so its versions win de-duplication, arguments are appended, and
// every scalar the child sets overrides the parent's.
func Merge(parent, child *Version) *Version {
	out := *parent
	out.ID, out.Type, out.InheritsFrom = child.ID, child.Type, ""
	if child.Type == "" {
		out.Type = parent.Type
	}
	if child.MainClass != "" {
		out.MainClass = child.MainClass
	}
	if child.MinecraftArguments != "" {
		out.MinecraftArguments = child.MinecraftArguments
	}
	if child.Arguments != nil {
		merged := Arguments{}
		if parent.Arguments != nil {
			merged = *parent.Arguments
			merged.Game = append([]Argument(nil), merged.Game...)
			merged.JVM = append([]Argument(nil), merged.JVM...)
		}
		merged.Game = append(merged.Game, child.Arguments.Game...)
		merged.JVM = append(merged.JVM, child.Arguments.JVM...)
		out.Arguments = &merged
	}
	if child.AssetIndex != nil {
		out.AssetIndex = child.AssetIndex
	}
	if child.Assets != "" {
		out.Assets = child.Assets
	}
	if len(child.Downloads) > 0 {
		out.Downloads = child.Downloads
	}
	if child.JavaVersion != nil {
		out.JavaVersion = child.JavaVersion
	}
	if child.Logging != nil {
		out.Logging = child.Logging
	}
	if !child.ReleaseTime.IsZero() {
		out.ReleaseTime = child.ReleaseTime
	}
	out.Libraries = append(append([]Library(nil), child.Libraries...), parent.Libraries...)
	return &out
}
