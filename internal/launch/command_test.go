package launch

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"quartz/internal/accounts"
	"quartz/internal/core/events"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/mojang"
	"quartz/internal/settings"
)

func TestCommandKeepsTokenOffTheCommandLine(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile("../mojang/testdata/1.21.4.json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := mojang.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	inst, err := insts.Create(instance.CreateRequest{Name: "T", Minecraft: "1.21.4"})
	if err != nil {
		t.Fatal(err)
	}
	inst, _ = insts.SetSettings(inst.ID, instance.Settings{
		Java:     instance.JavaSettings{Override: true, JVMArgs: `-Dfoo="a b" -XX:+UseZGC`},
		Window:   instance.WindowSettings{Width: 1280, Height: 720, Fullscreen: true},
		Commands: instance.CommandSettings{Wrapper: `nice -n "$NICE_LEVEL"`, Env: []instance.EnvVar{{Name: "MESA_GL_VERSION_OVERRIDE", Value: "4.6"}}},
	})
	t.Setenv("JAVA_TOOL_OPTIONS", "-javaagent:evil.jar")
	t.Setenv("NICE_LEVEL", "5")
	st, _ := settings.Open(filepath.Join(dir, "settings.json"), events.NewBus(), nil)
	l := New(Config{Instances: insts, Settings: st, BinDir: filepath.Join(dir, "bin")})
	res := &install.Resolved{
		Version: v, ClientJar: "/store/versions/1.21.4/1.21.4.jar", Classpath: []string{"/store/libraries/a.jar", "/store/versions/1.21.4/1.21.4.jar"},
		NativesDir: "/store/natives/x", AssetsRoot: "/store/assets", GameAssets: "/store/assets", LibrariesDir: "/store/libraries",
		LogConfig: "/store/logconfigs/client-1.12.xml", LogArgument: "-Dlog4j.configurationFile=${path}",
	}
	env := mojang.Env{OS: "linux", Arch: "x86_64", Features: map[string]bool{"has_custom_resolution": true}}
	sess := accounts.Session{Name: "Steve", UUID: "0123456789abcdef0123456789abcdef", AccessToken: "SECRET_TOKEN_eyJhbGciOi", UserType: "msa"}
	sp := spec{inst: inst, eff: inst.Resolve(st.Get()), sess: sess, res: res, rt: java.Runtime{Path: "/jdk/bin/java"}, mem: MemoryRange{Min: 1024, Max: 4096}, env: env, gameDir: insts.GameDir(inst.ID)}
	sp.vars = instVars(inst, insts.Dir(inst.ID), sp.gameDir, sp.rt.Path)
	cmd, script, _, err := l.command(&sp)
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(cmd.Argv, " ")
	if strings.Contains(argv, "SECRET_TOKEN") || strings.Contains(argv, "Steve") {
		t.Fatalf("game arguments leaked onto the command line: %s", argv)
	}
	for _, want := range []string{"-Xms1024M", "-Xmx4096M", "-Dfoo=a b", "-XX:+UseZGC", "-Dlog4j2.formatMsgNoLookups=true", "-Dstdout.encoding=UTF-8", "-Dsun.stderr.encoding=UTF-8", "-Dlog4j.configurationFile=/store/logconfigs/client-1.12.xml", "wrapper.Main"} {
		if !slices.Contains(cmd.Argv, want) {
			t.Errorf("missing %q in %v", want, cmd.Argv)
		}
	}
	if cmd.Argv[len(cmd.Argv)-1] != "wrapper.Main" {
		t.Fatal("wrapper must be the main class")
	}
	cp := cmd.Argv[slices.Index(cmd.Argv, "-cp")+1]
	if !strings.HasSuffix(cp, ".jar") || !strings.Contains(cp, "wrapper-") {
		t.Fatalf("classpath lacks the wrapper jar: %s", cp)
	}
	var game []string
	for _, line := range strings.Split(strings.TrimSpace(string(script)), "\n") {
		if cmd, val, _ := strings.Cut(line, " "); cmd == "arg" {
			b, _ := base64.StdEncoding.DecodeString(val)
			game = append(game, string(b))
		}
	}
	for _, want := range []string{"SECRET_TOKEN_eyJhbGciOi", "Steve", "--width", "1280", "--fullscreen"} {
		if !slices.Contains(game, want) {
			t.Errorf("game args lack %q: %v", want, game)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(string(script)), "launch "+base64.StdEncoding.EncodeToString([]byte(v.MainClass))) {
		t.Fatal("script must end by launching the game's main class")
	}
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "JAVA_TOOL_OPTIONS=") {
			t.Fatal("JAVA_TOOL_OPTIONS reached the game")
		}
	}
	if !slices.Contains(cmd.Env, "MESA_GL_VERSION_OVERRIDE=4.6") || !slices.Contains(cmd.Env, "INST_NAME=T") {
		t.Fatal("instance environment missing")
	}
	if !slices.Equal(cmd.Argv[:4], []string{"nice", "-n", "5", "/jdk/bin/java"}) {
		t.Fatalf("wrapper must come first, expanded per word: %v", cmd.Argv[:4])
	}
}

func TestCommandArgvNeverNeedsAShell(t *testing.T) {
	t.Setenv("EVIL", "; rm -rf ~")
	argv, err := commandArgv(`backup.sh "$INST_DIR" $EVIL`, map[string]string{"INST_DIR": "/data/My Pack"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(argv, []string{"backup.sh", "/data/My Pack", "; rm -rf ~"}) {
		t.Fatalf("argv = %q", argv)
	}
}

func TestBeforeLaunchCommandFailureStopsTheLaunch(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("needs /bin/sh")
	}
	err := runCommand(t.Context(), `/bin/sh -c "echo backup failed >&2; exit 3"`, t.TempDir(), nil, os.Environ())
	if err == nil || !strings.Contains(err.Error(), "backup failed") || !strings.Contains(err.Error(), "code 3") {
		t.Fatalf("err = %v", err)
	}
	if err := runCommand(t.Context(), `/bin/sh -c "exit 0"`, t.TempDir(), nil, os.Environ()); err != nil {
		t.Fatal(err)
	}
}

// LWJGL 2 runs xrandr to list display modes and crashes without it on Linux.
func TestLegacyLWJGLArgs(t *testing.T) {
	modern := &install.Resolved{Classpath: []string{"/l/org/lwjgl/lwjgl/3.3.3/lwjgl-3.3.3.jar"}}
	old := &install.Resolved{Classpath: []string{"/l/org/lwjgl/lwjgl/lwjgl/2.9.4-nightly-20150209/lwjgl-2.9.4-nightly-20150209.jar"}}
	if a := legacyLWJGLArgs(modern); a != nil {
		t.Fatalf("LWJGL 3 got %v", a)
	}
	_, err := exec.LookPath("xrandr")
	want := runtime.GOOS == "linux" && err != nil
	if got := slices.Contains(legacyLWJGLArgs(old), "-DLWJGL_DISABLE_XRANDR=true"); got != want {
		t.Fatalf("LWJGL 2: %v, want %v", got, want)
	}
	t.Setenv("PATH", t.TempDir())
	if runtime.GOOS == "linux" && !slices.Contains(legacyLWJGLArgs(old), "-DLWJGL_DISABLE_XRANDR=true") {
		t.Fatal("no xrandr on PATH, no flag")
	}
}
