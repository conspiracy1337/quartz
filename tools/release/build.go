package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// target is one release file.
type target struct {
	Platform string // in update.json: "linux-amd64", "darwin-universal", …
	File     string // in dist/
}

var targets = map[string]target{
	"linux-amd64":      {"linux-amd64", "quartz-linux-amd64"},
	"linux-arm64":      {"linux-arm64", "quartz-linux-arm64"},
	"windows-amd64":    {"windows-amd64", "quartz-windows-amd64.exe"},
	"windows-arm64":    {"windows-arm64", "quartz-windows-arm64.exe"},
	"darwin-universal": {"darwin-universal", "Quartz-macos.zip"},
}

// platformOf maps a dist file back to its platform.
func platformOf(file string) (string, bool) {
	for _, t := range targets {
		if t.File == file {
			return t.Platform, true
		}
	}
	return "", false
}

func defaultTargets() []string {
	if runtime.GOOS == "darwin" {
		return []string{"darwin-universal"}
	}
	return []string{"linux-amd64", "windows-amd64", "windows-arm64"}
}

// ldflags builds in the version, the update key and the service IDs.
func ldflags(version, pub string) string {
	set := func(name, v string) string { return fmt.Sprintf(" -X quartz/internal/brand.%s=%s", name, v) }
	f := "-s -w" + set("Version", version) + set("UpdatePublicKey", pub)
	for env, name := range map[string]string{"QUARTZ_MSA_CLIENT_ID": "MSAClientID", "QUARTZ_CF_API_KEY": "CurseForgeAPIKey", "QUARTZ_DISCORD_CLIENT_ID": "DiscordClientID"} {
		if v := os.Getenv(env); v != "" {
			f += set(name, v)
		}
	}
	return f
}

func build(version string, which []string) error {
	if !versionOK(version) {
		return fmt.Errorf("version %q isn't x.y.z", version)
	}
	pub, err := publicKey()
	if err != nil {
		return err
	}
	if len(which) == 0 {
		which = defaultTargets()
	}
	restore, err := setProductVersion(version)
	if err != nil {
		return err
	}
	defer restore()
	if err := os.MkdirAll("dist", 0o750); err != nil {
		return err
	}
	frontendBuilt := false
	for _, w := range which {
		t, ok := targets[w]
		if !ok {
			return fmt.Errorf("unknown target %q", w)
		}
		args := wailsArgs(w, ldflags(version, pub), t.File, frontendBuilt)
		fmt.Println("wails", strings.Join(args[:4], " "), "…")
		cmd := exec.Command("wails", args...) //nolint:gosec // fixed tool, our own arguments
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", w, err)
		}
		frontendBuilt = true
		if err := collect(w, t); err != nil {
			return err
		}
	}
	fmt.Println("Done. Next: release manifest", version, "<notes.md>")
	return nil
}

// wailsArgs is the wails build command line for one target. Release builds
// only ever carry the WebKitGTK tag: never the development tags (fakeauth,
// e2e) that swap in fake sign-in or scripted dialogs.
func wailsArgs(w, flags, file string, skipFrontend bool) []string {
	args := []string{"build", "-platform", strings.Replace(w, "-", "/", 1), "-trimpath", "-ldflags", flags, "-o", file}
	if strings.HasPrefix(w, "linux") {
		args = append(args, "-tags", "webkit2_41")
	}
	if skipFrontend {
		args = append(args, "-s") // the frontend is the same for every target
	}
	return args
}

// collect moves a build into dist/ (the macOS .app zipped, ad-hoc signed).
func collect(w string, t target) error {
	if w == "darwin-universal" {
		apps, _ := filepath.Glob(filepath.Join("build", "bin", "*.app"))
		if len(apps) != 1 {
			return errors.New("no .app in build/bin")
		}
		for _, c := range [][]string{
			{"codesign", "--force", "--deep", "-s", "-", apps[0]},
			{"ditto", "-c", "-k", "--keepParent", apps[0], filepath.Join("dist", t.File)},
		} {
			cmd := exec.Command(c[0], c[1:]...) //nolint:gosec // fixed tools
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("%s: %w", c[0], err)
			}
		}
		return nil
	}
	return os.Rename(filepath.Join("build", "bin", t.File), filepath.Join("dist", t.File))
}

// setProductVersion writes the version into wails.json for the build's
// resources (Windows file version, macOS Info.plist) and puts it back after.
func setProductVersion(version string) (func(), error) {
	orig, err := os.ReadFile("wails.json")
	if err != nil {
		return nil, fmt.Errorf("run this from the repository root: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(orig, &m); err != nil {
		return nil, err
	}
	info, _ := m["info"].(map[string]any)
	if info == nil {
		return nil, errors.New("wails.json has no info")
	}
	info["productVersion"] = version
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile("wails.json", append(b, '\n'), 0o644); err != nil { //nolint:gosec // a project file
		return nil, err
	}
	return func() { _ = os.WriteFile("wails.json", orig, 0o644) }, nil //nolint:gosec // a project file
}
