package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"quartz/internal/brand"
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

// serviceID is a build-time ID taken from the environment.
type serviceID struct {
	Env, Var string // environment variable, brand variable
	Valid    func(string) bool
	Feature  string // what is off without it
	Required bool   // a release without it misses a main feature
}

var serviceIDs = []serviceID{
	{brand.EnvMSAClientID, "MSAClientID", brand.ValidClientID, "Microsoft sign-in", true},
	{brand.EnvCurseForgeKey, "CurseForgeAPIKey", brand.ValidCurseForgeKey, "CurseForge", true},
	{brand.EnvDiscordClientID, "DiscordClientID", brand.ValidDiscordID, "Discord Rich Presence", false},
}

// checkIDs refuses malformed service IDs, and missing required ones unless
// allowMissing. A missing optional one is only reported.
func checkIDs(allowMissing bool) error {
	for _, id := range serviceIDs {
		v := os.Getenv(id.Env)
		switch {
		case v != "" && !id.Valid(v):
			return fmt.Errorf("%s is malformed", id.Env)
		case v == "" && id.Required && !allowMissing:
			return fmt.Errorf("%s is not set, so %s would be off in this release (build -allow-missing to accept that)", id.Env, id.Feature)
		case v == "":
			fmt.Fprintf(os.Stderr, "warning: %s is not set: %s is off in this release\n", id.Env, id.Feature)
		}
	}
	return nil
}

// setVar is the linker flag that sets one brand variable.
func setVar(name, v string) string {
	return fmt.Sprintf(" -X quartz/internal/brand.%s=%s", name, v)
}

// serviceFlags builds in the service IDs that are set (checked by checkIDs
// first).
func serviceFlags() string {
	f := ""
	for _, id := range serviceIDs {
		if v := os.Getenv(id.Env); v != "" {
			f += setVar(id.Var, v)
		}
	}
	return f
}

// ldflags builds in the version, the update key and the service IDs.
func ldflags(version, pub string) string {
	return "-s -w" + setVar("Version", version) + setVar("UpdatePublicKey", pub) + serviceFlags()
}

// devFlags prints the service IDs from .env and the environment as linker
// flags for development builds (build.sh, wails dev). They carry no version
// and no update key, so a development build never updates itself.
func devFlags() error {
	if err := loadDotEnv(dotEnv); err != nil {
		return err
	}
	if err := checkIDs(true); err != nil {
		return err
	}
	fmt.Println(strings.TrimSpace(serviceFlags()))
	return nil
}

func build(ctx context.Context, version string, which []string, allowMissing bool) error {
	if !versionOK(version) {
		return fmt.Errorf("version %q isn't x.y.z", version)
	}
	if err := loadDotEnv(dotEnv); err != nil {
		return err
	}
	if err := checkIDs(allowMissing); err != nil {
		return err
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
		if err := runWails(ctx, args...); err != nil {
			return fmt.Errorf("%s: %w", w, err)
		}
		frontendBuilt = true
		if err := collect(ctx, w, t); err != nil {
			return err
		}
		out := filepath.Join("dist", t.File)
		summary, err := verifyArtifact(out)
		if err != nil {
			_ = os.Remove(out)
			return err
		}
		fmt.Println(summary)
		if err := stamp("dist", t.File, version); err != nil {
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

// runWails runs wails with the app icon lent from assets/. Windows and
// macOS builds read build/appicon.png (Wails writes its own default icon
// there when it is missing) and make build/windows/*.ico from it, so the
// icon is put there for the run only and everything made from it is removed
// after, leaving assets/appicon.png the one copy.
func runWails(ctx context.Context, args ...string) error {
	icon, err := os.ReadFile(filepath.Join("assets", "appicon.png"))
	if err != nil {
		return fmt.Errorf("run this from the repository root: %w", err)
	}
	lent := []string{filepath.Join("build", "appicon.png"), filepath.Join("build", "windows", "icon.ico"), filepath.Join("build", "windows", "appicon.ico")}
	clean := func() {
		for _, f := range lent {
			_ = os.Remove(f)
		}
	}
	clean() // a stale .ico would win over the icon
	defer clean()
	if err := os.WriteFile(lent[0], icon, 0o644); err != nil { //nolint:gosec // build input
		return err
	}
	return runTool(ctx, "wails", args...)
}

// runTool runs a fixed build tool with our own arguments, stopped by ctx.
func runTool(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // fixed tools, our own arguments
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// collect moves a build into dist/ (the macOS .app zipped, ad-hoc signed).
func collect(ctx context.Context, w string, t target) error {
	if w == "darwin-universal" {
		apps, _ := filepath.Glob(filepath.Join("build", "bin", "*.app"))
		if len(apps) != 1 {
			return errors.New("no .app in build/bin")
		}
		for _, c := range [][]string{
			{"codesign", "--force", "--deep", "-s", "-", apps[0]},
			{"ditto", "-c", "-k", "--keepParent", apps[0], filepath.Join("dist", t.File)},
		} {
			if err := runTool(ctx, c[0], c[1:]...); err != nil {
				return fmt.Errorf("%s: %w", c[0], err)
			}
		}
		return nil
	}
	return os.Rename(filepath.Join("build", "bin", t.File), filepath.Join("dist", t.File))
}

// stampFile records which version built each file in dist/, so a manifest
// never signs a file left over from an older release. It is a dotfile, so
// publishing dist/* leaves it out.
const stampFile = ".builds.json"

func readStamps(dir string) map[string]string {
	stamps := map[string]string{}
	if b, err := os.ReadFile(filepath.Join(dir, stampFile)); err == nil { //nolint:gosec // our own output
		_ = json.Unmarshal(b, &stamps)
	}
	return stamps
}

func stamp(dir, file, version string) error {
	stamps := readStamps(dir)
	stamps[file] = version
	b, err := json.MarshalIndent(stamps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, stampFile), append(b, '\n'), 0o644) //nolint:gosec // build bookkeeping
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
