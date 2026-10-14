package crash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/logging"
	"quartz/internal/gamelog"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRules(t *testing.T) {
	cases := []struct {
		name, report, log string
		want              Finding
	}{
		{"fabric mixin report", fixture(t, "fabric-mixin.txt"), "", Finding{Rule: "mixin", ModID: "entityculling", File: "entityculling-fabric-1.7.2-mc1.21.jar"}},
		{"fabric mixin log", "", "Mixin apply for mod entityculling failed entityculling.mixins.json:EntityRendererMixin from mod entityculling -> net.minecraft.class_897: org.spongepowered.asm.mixin.injection.throwables.InvalidInjectionException", Finding{Rule: "mixin", ModID: "entityculling", Target: "net.minecraft.class_897:"}},
		{"forge deps", fixture(t, "forge-deps.txt"), "", Finding{Rule: "dependency", ModID: "roughlyenoughitems", Dep: "architectury"}},
		{"fabric deps", "", "net.fabricmc.loader.impl.FormattedException: Mod resolution encountered an incompatible mod set!\nMore details:\n\t - Mod 'Mod Menu' (modmenu) 11.0.3 requires any version of fabric-api, which is missing!", Finding{Rule: "dependency", ModID: "modmenu", Dep: "fabric-api"}},
		{"forge mod", fixture(t, "forge-mod.txt"), "", Finding{Rule: "mod", ModID: "embeddium", File: "embeddium-0.3.18+mc1.20.1.jar", Text: "Embeddium (embeddium) has failed to load correctly"}},
		{"duplicate", "", "Found duplicate mods:\n\tMod ID: 'jei' from mod files: jei-1.20.1-forge-15.3.0.4.jar, jei-1.20.1-forge-15.2.0.27.jar", Finding{Rule: "duplicate", ModID: "jei", File: "jei-1.20.1-forge-15.2.0.27.jar"}},
		{"java", "", "java.lang.UnsupportedClassVersionError: net/minecraft/client/main/Main has been compiled by a more recent version of the Java Runtime (class file version 65.0), this version of the Java Runtime only recognizes class file versions up to 61.0", Finding{Rule: "java", Java: 21, Ran: 17}},
		{"memory", "", "java.lang.OutOfMemoryError: Java heap space", Finding{Rule: "memory"}},
		{"graphics", "", "GLFW error 65542: WGL: The driver does not appear to support OpenGL", Finding{Rule: "graphics"}},
		{"fabric minecraft version", "", fixture(t, "fabric-version.txt"), Finding{Rule: "version", ModID: "sodium", Need: "1.20.4", Have: "1.20.1"}},
		{"fabric dependency version", "", "\t - Mod 'Roughly Enough Items' (roughlyenoughitems) 16.0.729 requires version 13.0.6 or later of mod 'Architectury' (architectury), but only the wrong version is present: 12.1.4!",
			Finding{Rule: "dependency", ModID: "roughlyenoughitems", Dep: "architectury", Need: "13.0.6 or later", Have: "12.1.4"}},
		{"fabric old loader version", "", "Mod 'Iris' (iris) 1.6.4 requires any 1.20.x version of minecraft, but only the wrong version is present: 1.19.4!", Finding{Rule: "version", ModID: "iris", Need: "1.20.x", Have: "1.19.4"}},
		{"fabric conflict", "", "\t - Mod 'OptiFabric' (optifabric) 1.13.0 is incompatible with any version of mod 'Sodium' (sodium), but a matching version is present: 0.5.3!", Finding{Rule: "conflict", ModID: "optifabric", Dep: "sodium"}},
		{"java too new for old forge", "", "java.lang.IllegalArgumentException: Unsupported class file major version 65", Finding{Rule: "java-new", Ran: 21}},
		{"java too new for modlauncher", "", "java.lang.ClassCastException: class jdk.internal.loader.ClassLoaders$AppClassLoader cannot be cast to class java.net.URLClassLoader", Finding{Rule: "java-new"}},
		{"heap", "", "Error occurred during initialization of VM\nCould not reserve enough space for 8388608KB object heap", Finding{Rule: "jvm-memory"}},
		{"vm option", "", "Unrecognized VM option 'UseZGCC'\nError: Could not create the Java Virtual Machine.", Finding{Rule: "jvm-arg", Arg: "UseZGCC"}},
		{"java option", "", "Unrecognized option: --enable-preveiw", Finding{Rule: "jvm-arg", Arg: "--enable-preveiw"}},
		{"native driver", fixture(t, "hs_err-nvidia.log"), "", Finding{Rule: "graphics", Arg: "nvoglv64.dll"}},
		{"native other", "# Problematic frame:\n# C  [lwjgl.dll+0x1234]\n", "", Finding{Rule: "native", Arg: "lwjgl.dll"}},
		{"native memory", "# There is insufficient memory for the Java Runtime Environment to continue.\n", "", Finding{Rule: "memory"}},
	}
	for _, c := range cases {
		got, ok := Find(c.report, c.log)
		if !ok {
			t.Errorf("%s: nothing found", c.name)
			continue
		}
		if got.Rule != c.want.Rule || got.ModID != c.want.ModID || got.Dep != c.want.Dep || got.Java != c.want.Java || got.Ran != c.want.Ran ||
			got.Need != c.want.Need || got.Have != c.want.Have || got.Arg != c.want.Arg ||
			(c.want.File != "" && got.File != c.want.File) || (c.want.Text != "" && got.Text != c.want.Text) || (c.want.Target != "" && got.Target != c.want.Target) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
	if _, ok := Find("", "[main/INFO]: Stopping!"); ok {
		t.Error("a clean exit matched a rule")
	}
}

func TestTrace(t *testing.T) {
	tr := Trace(fixture(t, "fabric-mixin.txt"), "")
	lines := strings.Split(tr, "\n")
	if !strings.HasPrefix(lines[0], "java.lang.RuntimeException: Mixin transformation") || len(lines) > traceLines {
		t.Fatalf("%q", tr)
	}
	if !strings.Contains(tr, "Caused by: org.spongepowered.asm.mixin.injection.throwables.InvalidInjectionException") || strings.Count(tr, "at net.fabricmc.loader.impl.launch.knot.KnotClassDelegate") > 3 {
		t.Fatalf("trace: %s", tr)
	}
	if !strings.Contains(Trace(fixture(t, "forge-mod.txt"), ""), "Mod File: /home/steve/.minecraft/mods/embeddium") {
		t.Fatal("mod file line missing")
	}
}

type fakeMods map[string]content.Row

func (f fakeMods) ByModID(_ context.Context, _, modID, file string) (content.Row, bool) {
	if r, ok := f[modID]; ok {
		return r, true
	}
	r, ok := f[file]
	return r, ok
}

type fakeRuns struct{ dir string }

func (f fakeRuns) Run(_, runID string) (launch.Record, error) {
	return launch.Record{ID: runID, LengthMin: 3, ExitCode: 1, Result: launch.ResultCrash}, nil
}

func (f fakeRuns) Evidence(_, runID, kind string) (string, error) {
	p, err := launch.EvidencePath(f.dir, runID, kind)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

func TestFinishAndExplain(t *testing.T) {
	dir := t.TempDir()
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	inst, _ := insts.Create(instance.CreateRequest{Name: "FO", Minecraft: "1.21.4"})
	game := insts.GameDir(inst.ID)
	runs := insts.RunsDir(inst.ID)
	_ = os.MkdirAll(filepath.Join(game, "crash-reports"), 0o700)
	_ = os.MkdirAll(runs, 0o700)
	home, _ := os.UserHomeDir()
	report := strings.ReplaceAll(fixture(t, "fabric-mixin.txt"), "Eclipse Adoptium", "Eclipse Adoptium at "+home+"/x")
	_ = os.WriteFile(filepath.Join(game, "crash-reports", "crash-2025-03-21_19.33.40-client.txt"), []byte(report), 0o600)

	mods := fakeMods{"entityculling": {ID: "abc123", Name: "Entity Culling", Version: "1.7.2", Enabled: true, Status: &content.Status{Kind: "update", Version: "1.7.4"}}}
	svc := New(Deps{Instances: insts, Mods: mods, Runs: fakeRuns{runs}, Redactor: logging.NewRedactor(filepath.Base(home))})
	rec := launch.Record{ID: "20250321-193340.000", Result: launch.ResultCrash}
	svc.FinishRun(&launch.Finished{InstanceID: inst.ID, GameDir: game, RunsDir: runs, Started: time.Now().Add(-time.Minute), Record: &rec, Tail: []gamelog.Line{{Message: "Stopping!"}}})
	if rec.Crash == nil || rec.Crash.Title != "Entity Culling 1.7.2 failed to load" || rec.Crash.Detail != "Mixin apply failed in entityculling" || !rec.Crash.Report {
		t.Fatalf("crash %+v", rec.Crash)
	}
	saved, err := os.ReadFile(filepath.Join(runs, rec.ID+".crash.txt"))
	if err != nil || (home != "" && strings.Contains(string(saved), home)) {
		t.Fatalf("saved report not redacted: %v", err)
	}
	info, err := svc.ForRun(context.Background(), inst.ID, rec.ID)
	if err != nil || info.Cause == nil || info.Cause.Mod == nil || info.Cause.Mod.Update != "1.7.4" {
		t.Fatalf("%+v %v", info, err)
	}
	if strings.Join(info.Cause.Fixes, ",") != "update,disable" || !strings.Contains(info.Cause.Text, "Entity Culling 1.7.4 is out") {
		t.Fatalf("cause %+v", info.Cause)
	}
	if info.LengthMin != 3 || info.ExitCode != 1 || info.Trace == "" {
		t.Fatalf("%+v", info)
	}
	if _, err := svc.ForFile(context.Background(), inst.ID, "../options.txt"); err == nil {
		t.Fatal("bad report name accepted")
	}
	fi, err := svc.ForFile(context.Background(), inst.ID, "crash-2025-03-21_19.33.40-client.txt")
	if err != nil || fi.Cause == nil || fi.File == "" {
		t.Fatalf("%+v %v", fi, err)
	}
	// A clean exit keeps no cause, even with an old report lying around.
	clean := launch.Record{ID: "20250321-200000.000", Result: launch.ResultOK}
	svc.FinishRun(&launch.Finished{InstanceID: inst.ID, GameDir: game, RunsDir: runs, Started: time.Now().Add(time.Hour), Record: &clean})
	if clean.Crash != nil {
		t.Fatalf("clean run got %+v", clean.Crash)
	}
}

// A crash report makes a cleanly exited run a crash (Forge's loading-error
// screen exits with 0), unless the player stopped it. Without a report, the
// JVM's crash log is kept, cut before its environment.
func TestFinishFindsCrashes(t *testing.T) {
	dir := t.TempDir()
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	inst, _ := insts.Create(instance.CreateRequest{Name: "FO", Minecraft: "1.20.1"})
	game, runs := insts.GameDir(inst.ID), insts.RunsDir(inst.ID)
	_ = os.MkdirAll(filepath.Join(game, "crash-reports"), 0o700)
	_ = os.MkdirAll(runs, 0o700)
	svc := New(Deps{Instances: insts, Mods: fakeMods{}, Runs: fakeRuns{runs}, Redactor: logging.NewRedactor("steve")})
	start := time.Now().Add(-time.Minute)
	_ = os.WriteFile(filepath.Join(game, "crash-reports", "crash-2026-10-14_12.00.00-fml.txt"), []byte(fixture(t, "forge-deps.txt")), 0o600)

	stopped := launch.Record{ID: "20261014-120000.000", Result: launch.ResultOK}
	svc.FinishRun(&launch.Finished{InstanceID: inst.ID, GameDir: game, RunsDir: runs, Started: start, Record: &stopped, Stopped: true})
	if stopped.Result != launch.ResultOK {
		t.Fatalf("a stopped run became %s", stopped.Result)
	}
	clean := launch.Record{ID: "20261014-120001.000", Result: launch.ResultOK}
	svc.FinishRun(&launch.Finished{InstanceID: inst.ID, GameDir: game, RunsDir: runs, Started: start, Record: &clean})
	if clean.Result != launch.ResultCrash || clean.Crash == nil || clean.Crash.Title != "roughlyenoughitems needs architectury" {
		t.Fatalf("exit 0 with a report: %s %+v", clean.Result, clean.Crash)
	}

	_ = os.RemoveAll(filepath.Join(game, "crash-reports"))
	_ = os.WriteFile(filepath.Join(game, "hs_err_pid1.log"), []byte(fixture(t, "hs_err-nvidia.log")), 0o600)
	native := launch.Record{ID: "20261014-120002.000", Result: launch.ResultCrash, ExitCode: 1}
	svc.FinishRun(&launch.Finished{InstanceID: inst.ID, GameDir: game, RunsDir: runs, Started: start, Record: &native})
	if native.Crash == nil || native.Crash.Title != "The graphics driver crashed" || !native.Crash.Report {
		t.Fatalf("native crash %+v", native.Crash)
	}
	saved, _ := os.ReadFile(filepath.Join(runs, native.ID+".crash.txt"))
	if strings.Contains(string(saved), "USERNAME") || !strings.Contains(string(saved), "nvoglv64.dll") {
		t.Fatalf("saved native log: %s", saved)
	}
	info, err := svc.ForRun(context.Background(), inst.ID, native.ID)
	if err != nil || !strings.Contains(info.Trace, "EXCEPTION_ACCESS_VIOLATION") || !strings.Contains(info.Trace, "[nvoglv64.dll") {
		t.Fatalf("trace %q %v", info.Trace, err)
	}
}
