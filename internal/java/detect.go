package java

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// exe is the console Java binary name used for probing.
func exe(goos string) string {
	if goos == "windows" {
		return "java.exe"
	}
	return "java"
}

// LaunchBinary prefers javaw.exe on Windows so no console window opens.
func LaunchBinary(javaPath string) string {
	if runtime.GOOS != "windows" {
		return javaPath
	}
	w := filepath.Join(filepath.Dir(javaPath), "javaw.exe")
	if _, err := os.Stat(w); err == nil {
		return w
	}
	return javaPath
}

// searchEnv is what detection depends on, injected for tests.
type searchEnv struct {
	goos   string
	home   string
	getenv func(string) string
}

// candidate is a java binary and where it was found.
type candidate struct {
	path   string
	source Source
}

// candidates lists java binaries in every place Prism looks (JavaUtils.cpp),
// plus JAVA_HOME and the official launcher's bundled runtimes.
func candidates(e searchEnv) []candidate {
	var out []candidate
	add := func(src Source, paths ...string) {
		for _, p := range paths {
			out = append(out, candidate{path: p, source: src})
		}
	}
	bin := exe(e.goos)
	// Each JDK folder inside these roots holds bin/java (or jre/bin/java).
	scan := func(src Source, roots ...string) {
		for _, root := range roots {
			entries, err := os.ReadDir(root)
			if err != nil {
				continue
			}
			for _, d := range entries {
				base := filepath.Join(root, d.Name())
				add(src, filepath.Join(base, "bin", bin), filepath.Join(base, "jre", "bin", bin),
					filepath.Join(base, "Contents", "Home", "bin", bin))
			}
		}
	}
	envOr := func(name, def string) string {
		if v := e.getenv(name); v != "" {
			return v
		}
		return def
	}
	h := e.home

	switch e.goos {
	case "linux", "freebsd", "openbsd", "netbsd":
		roots := []string{"/usr/java", "/usr/lib/jvm", "/usr/lib64/jvm", "/usr/lib32/jvm", "/opt/jdk", "/opt/jdks", "/opt/java", "/opt/ibm", "/app/jdk", "/usr/local/openjdk"}
		if snap := e.getenv("SNAP"); snap != "" {
			for _, r := range roots[:4] {
				roots = append(roots, filepath.Join(snap, r))
			}
		}
		scan(System, roots...)
		add(System, "/app/jdk/bin/java", "/opt/jdk/bin/java")
	case "darwin":
		scan(System, "/Library/Java/JavaVirtualMachines", "/System/Library/Java/JavaVirtualMachines", filepath.Join(h, "Library/Java/JavaVirtualMachines"),
			"/opt/homebrew/opt", "/usr/local/opt")
		add(System, "/usr/bin/java")
	case "windows":
		for _, pf := range []string{e.getenv("ProgramFiles"), e.getenv("ProgramFiles(x86)"), e.getenv("ProgramW6432")} {
			if pf == "" {
				continue
			}
			for _, vendor := range []string{"Java", "Eclipse Adoptium", "Eclipse Foundation", "AdoptOpenJDK", "Zulu", "Microsoft", "BellSoft", "Semeru", "Amazon Corretto"} {
				scan(System, filepath.Join(pf, vendor))
			}
		}
		add(System, registryJavaHomes()...)
	}

	scan(SDKMAN, filepath.Join(envOr("SDKMAN_DIR", filepath.Join(h, ".sdkman")), "candidates", "java"))
	scan(Found, filepath.Join(envOr("ASDF_DATA_DIR", filepath.Join(h, ".asdf")), "installs", "java"),
		filepath.Join(h, ".jdks"), filepath.Join(envOr("GRADLE_USER_HOME", filepath.Join(h, ".gradle")), "jdks"))
	if jh := e.getenv("JAVA_HOME"); jh != "" {
		add(Found, filepath.Join(jh, "bin", bin))
	}
	for _, dir := range filepath.SplitList(e.getenv("PATH")) {
		if dir != "" {
			add(System, filepath.Join(dir, bin))
		}
	}
	for _, rt := range officialRuntimeDirs(e) {
		add(Found, findBinaries(rt, bin, 4)...)
	}
	return out
}

// officialRuntimeDirs are where the official launcher keeps its Java runtimes.
func officialRuntimeDirs(e searchEnv) []string {
	switch e.goos {
	case "windows":
		var dirs []string
		if a := e.getenv("APPDATA"); a != "" {
			dirs = append(dirs, filepath.Join(a, ".minecraft", "runtime"))
		}
		if l := e.getenv("LOCALAPPDATA"); l != "" {
			dirs = append(dirs, filepath.Join(l, "Packages", "Microsoft.4297127D64EC6_8wekyb3d8bbwe", "LocalCache", "Local", "runtime"))
		}
		return dirs
	case "darwin":
		return []string{filepath.Join(e.home, "Library", "Application Support", "minecraft", "runtime")}
	default:
		return []string{filepath.Join(e.home, ".minecraft", "runtime")}
	}
}

// findBinaries walks root (to a small depth) for bin/<name>.
func findBinaries(root, name string, depth int) []string {
	var out []string
	base := strings.Count(filepath.Clean(root), string(filepath.Separator))
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable folders are simply skipped
		}
		if d.IsDir() && strings.Count(p, string(filepath.Separator))-base > depth {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == name && filepath.Base(filepath.Dir(p)) == "bin" {
			out = append(out, p)
		}
		return nil
	})
	return out
}
