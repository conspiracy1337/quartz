package launch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"quartz/internal/core/procx"
	"quartz/internal/core/shlex"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/sandbox"
)

// commandTimeout bounds the player's before-launch and after-exit commands.
const commandTimeout = 10 * time.Minute

// instVars are the $INST_* values commands and the game's environment get
// (Prism's names, so existing scripts keep working).
func instVars(inst instance.Instance, instDir, gameDir, javaPath string) map[string]string {
	return map[string]string{
		"INST_ID": inst.ID, "INST_NAME": inst.Name, "INST_DIR": instDir, "INST_MC_DIR": gameDir,
		"INST_JAVA": javaPath, "INST_MC_VERSION": inst.Minecraft, "INST_LOADER": inst.Loader,
	}
}

// commandArgv splits a command into words like a shell would, then expands
// $VAR and ${VAR} in each word from vars and the environment. Nothing ever
// runs through a shell, so an expanded value can't add words or operators;
// to use shell features, write `sh -c '…'` explicitly.
func commandArgv(line string, vars map[string]string) ([]string, error) {
	words, err := shlex.Split(line)
	if err != nil {
		return nil, err
	}
	lookup := func(name string) string {
		if v, ok := vars[name]; ok {
			return v
		}
		return os.Getenv(name)
	}
	for i, w := range words {
		words[i] = os.Expand(w, lookup)
	}
	return words, nil
}

// envList turns vars into KEY=value entries.
func envList(vars map[string]string) []string {
	out := make([]string, 0, len(vars))
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	return out
}

// gameEnv is the game's environment: the cleaned launcher environment (no
// JVM-flag injection; only the sandbox's allowlist when sandboxed), the
// $INST_* values, the Linux performance switches and the instance's own
// variables last, so they win.
func gameEnv(vars map[string]string, eff instance.Effective, sandboxed bool) []string {
	base := os.Environ()
	if sandboxed {
		base = sandbox.FilterEnv(base)
	}
	env := append(java.CleanEnv(base), envList(vars)...)
	if runtime.GOOS == "linux" && eff.Performance.DedicatedGPU {
		env = append(env, "DRI_PRIME=1")
		if _, err := os.Stat("/proc/driver/nvidia"); err == nil {
			env = append(env, "__NV_PRIME_RENDER_OFFLOAD=1", "__GLX_VENDOR_LIBRARY_NAME=nvidia", "__VK_LAYER_NV_optimus=NVIDIA_only")
		}
	}
	for _, e := range eff.Commands.Env {
		env = append(env, e.Name+"="+e.Value)
	}
	return env
}

// wrappers returns the programs put in front of java: the player's wrapper,
// then GameMode and MangoHud when enabled and installed (Linux only). A
// sandboxed game has no session bus for gamemoderun; the sandbox registers
// it with GameMode from outside instead.
func wrappers(eff instance.Effective, vars map[string]string, sandboxed bool) ([]string, error) {
	var out []string
	if eff.Commands.Wrapper != "" {
		w, err := commandArgv(eff.Commands.Wrapper, vars)
		if err != nil {
			return nil, fmt.Errorf("wrapper command: %w", err)
		}
		out = append(out, w...)
	}
	if runtime.GOOS == "linux" {
		for _, t := range []struct {
			on  bool
			bin string
		}{{eff.Performance.GameMode && !sandboxed, "gamemoderun"}, {eff.Performance.MangoHud, "mangohud"}} {
			if t.on {
				if p, err := exec.LookPath(t.bin); err == nil {
					out = append(out, p)
				}
			}
		}
	}
	return out, nil
}

// nativeLibraryArgs points LWJGL at the system's GLFW and OpenAL (Linux).
func nativeLibraryArgs(eff instance.Effective) []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	var args []string
	if eff.Commands.SystemGLFW {
		if p := findSystemLibrary("libglfw.so.3"); p != "" {
			args = append(args, "-Dorg.lwjgl.glfw.libname="+p)
		}
	}
	if eff.Commands.SystemOpenAL {
		if p := findSystemLibrary("libopenal.so.1"); p != "" {
			args = append(args, "-Dorg.lwjgl.openal.libname="+p)
		}
	}
	return args
}

var libraryDirs = []string{"/usr/lib64", "/usr/lib", "/usr/lib/x86_64-linux-gnu", "/usr/lib/aarch64-linux-gnu", "/lib64", "/lib"}

func findSystemLibrary(name string) string {
	for _, d := range libraryDirs {
		p := filepath.Join(d, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// runCommand runs one of the player's commands to completion and returns its
// last lines on failure.
func runCommand(ctx context.Context, line, dir string, vars map[string]string, env []string) error {
	argv, err := commandArgv(line, vars)
	if err != nil || len(argv) == 0 {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // the player's own command, argv only, no shell
	cmd.Dir, cmd.Env, cmd.WaitDelay = dir, env, 5*time.Second
	procx.NoConsole(cmd)
	out := &procx.Tail{Max: 8 << 10}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := out.Last(3)
		if code := exitCode(cmd); code != "" {
			return fmt.Errorf("%s exited with %s: %s", filepath.Base(argv[0]), code, msg)
		}
		return fmt.Errorf("%s: %w", filepath.Base(argv[0]), err)
	}
	return nil
}

func exitCode(cmd *exec.Cmd) string {
	if cmd.ProcessState == nil {
		return ""
	}
	return "code " + strconv.Itoa(cmd.ProcessState.ExitCode())
}
