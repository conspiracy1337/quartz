package mojang

// RuntimeIndexURL lists Mojang's Java runtimes per platform and component.
const RuntimeIndexURL = "https://launchermeta.mojang.com/v1/products/java-runtime/2ec0cc96c44e5a76b9c8b7c39df7210883d12871/all.json"

// RuntimeIndex maps platform → component → available builds.
type RuntimeIndex map[string]map[string][]RuntimeBuild

// RuntimeBuild is one build of a runtime component.
type RuntimeBuild struct {
	Manifest Artifact `json:"manifest"`
	Version  struct {
		Name string `json:"name"`
	} `json:"version"`
}

// RuntimeManifest lists every file of one runtime build.
type RuntimeManifest struct {
	Files map[string]RuntimeFile `json:"files"`
}

// RuntimeFile is a file, directory or link inside a runtime.
type RuntimeFile struct {
	Type       string              `json:"type"` // file | directory | link
	Executable bool                `json:"executable,omitempty"`
	Target     string              `json:"target,omitempty"`
	Downloads  map[string]Artifact `json:"downloads,omitempty"` // raw, lzma
}

// RuntimePlatform maps GOOS/GOARCH to Mojang's platform key; "" when Mojang
// publishes nothing for it (Linux arm64).
func RuntimePlatform(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "linux"
	case "linux/386":
		return "linux-i386"
	case "darwin/amd64":
		return "mac-os"
	case "darwin/arm64":
		return "mac-os-arm64"
	case "windows/amd64":
		return "windows-x64"
	case "windows/386":
		return "windows-x86"
	case "windows/arm64":
		return "windows-arm64"
	}
	return ""
}

// Build returns the first build of component for platform. Apple silicon
// falls back to the Intel build (Rosetta) when Mojang lacks a native one.
func (ix RuntimeIndex) Build(platform, component string) (RuntimeBuild, bool) {
	if b := ix[platform][component]; len(b) > 0 {
		return b[0], true
	}
	if platform == "mac-os-arm64" {
		return ix.Build("mac-os", component)
	}
	return RuntimeBuild{}, false
}
