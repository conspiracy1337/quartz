package sandbox

import (
	"runtime"
	"strings"
)

// envNames pass into a sandbox: what the game needs to find its display,
// audio, locale and system. Everything else in the launcher's environment
// (agent sockets, the session bus, keyring and cloud tokens…) stays out.
var envNames = map[string]bool{
	"PATH": true, "LANG": true, "LANGUAGE": true, "TZ": true, "TERM": true, "HOME": true, "USER": true, "LOGNAME": true, "USERNAME": true,
	"DISPLAY": true, "XAUTHORITY": true, "WAYLAND_DISPLAY": true, "XDG_RUNTIME_DIR": true, "XDG_SESSION_TYPE": true,
	"XDG_CURRENT_DESKTOP": true, "XDG_SESSION_DESKTOP": true, "DESKTOP_SESSION": true, "PULSE_SERVER": true,
	"TMPDIR": true, "__CF_USER_TEXT_ENCODING": true,
	// Windows
	"SYSTEMROOT": true, "WINDIR": true, "SYSTEMDRIVE": true, "TEMP": true, "TMP": true, "APPDATA": true, "LOCALAPPDATA": true,
	"USERPROFILE": true, "PROGRAMFILES": true, "PROGRAMFILES(X86)": true, "PROGRAMDATA": true, "COMMONPROGRAMFILES": true,
	"NUMBER_OF_PROCESSORS": true, "PROCESSOR_ARCHITECTURE": true, "PROCESSOR_IDENTIFIER": true, "OS": true, "PATHEXT": true, "COMSPEC": true,
}

// envPrefixes pass too: locale, graphics drivers and overlays.
var envPrefixes = []string{"LC_", "__GL_", "__NV_", "__GLX_", "MESA_", "LIBGL_", "DRI_", "VK_", "MANGOHUD", "SDL_", "GTK_THEME", "QT_"}

// FilterEnv keeps only the variables a sandboxed game may see.
func FilterEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		key := name
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(name)
		}
		if envNames[key] || hasPrefix(key) {
			out = append(out, kv)
		}
	}
	return out
}

func hasPrefix(name string) bool {
	for _, p := range envPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// setEnv returns env with each KEY=value of add replacing an earlier one.
func setEnv(env []string, add ...string) []string {
	for _, kv := range add {
		name, _, _ := strings.Cut(kv, "=")
		out := env[:0:0]
		for _, e := range env {
			if n, _, _ := strings.Cut(e, "="); n != name {
				out = append(out, e)
			}
		}
		env = append(out, kv)
	}
	return env
}
