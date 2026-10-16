//go:build cgo

package webview

/*
#cgo pkg-config: glib-2.0
#include <glib.h>
#include <stdlib.h>
#include <string.h>

static char *swap(const char *key, const char *val) {
	const char *old = getenv(key);
	char *saved = old ? strdup(old) : NULL;
	setenv(key, val, 1);
	return saved;
}

static void restore(const char *key, char *saved) {
	if (saved) {
		setenv(key, saved, 1);
		free(saved);
	} else {
		unsetenv(key);
	}
}

// isolate names the program, then has glib read its user data and cache
// folders as dir. glib keeps both for the life of the process, so the
// variables are put back at once: nothing the launcher starts sees them.
// Only the C environment is touched; Go's copy never changes.
static int isolate(const char *dir, const char *prgname) {
	g_set_prgname(prgname);
	char *data = swap("XDG_DATA_HOME", dir);
	char *cache = swap("XDG_CACHE_HOME", dir);
	int ok = strcmp(g_get_user_data_dir(), dir) == 0 && strcmp(g_get_user_cache_dir(), dir) == 0;
	restore("XDG_DATA_HOME", data);
	restore("XDG_CACHE_HOME", cache);
	return ok;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Isolate puts the webview's storage and cache in dir and names the program
// (the Wayland app_id). It must run before the window is made, while nothing
// else runs C code.
func Isolate(dir, program string) error {
	d, p := C.CString(dir), C.CString(program)
	defer C.free(unsafe.Pointer(d))
	defer C.free(unsafe.Pointer(p))
	if C.isolate(d, p) == 0 {
		return errors.New("webview: glib had already read its folders")
	}
	return nil
}

// cGetenv reads the C environment, which Go doesn't mirror back.
func cGetenv(key string) (string, bool) {
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	v := C.getenv(k)
	if v == nil {
		return "", false
	}
	return C.GoString(v), true
}
