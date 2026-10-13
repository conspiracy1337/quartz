//go:build e2e

package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// An end-to-end build (-tags e2e) answers the native dialogs from the folder
// QUARTZ_E2E_DIALOGS names, so a browser driving the dev server can pick
// files: a save lands in that folder under its suggested name, an open
// returns the path written in its "open" file, and a folder pick the path in
// its "folder" file. Without the variable the real dialogs open. Release
// builds never carry this file.
func init() {
	realOpen, realDir, realSave := openFileDialog, openDirDialog, saveFileDialog
	openFileDialog = func(ctx context.Context, o wruntime.OpenDialogOptions) (string, error) {
		if dir := e2eDialogs(); dir != "" {
			return e2eAnswer(dir, "open"), nil
		}
		return realOpen(ctx, o)
	}
	openDirDialog = func(ctx context.Context, o wruntime.OpenDialogOptions) (string, error) {
		if dir := e2eDialogs(); dir != "" {
			return e2eAnswer(dir, "folder"), nil
		}
		return realDir(ctx, o)
	}
	saveFileDialog = func(ctx context.Context, o wruntime.SaveDialogOptions) (string, error) {
		if dir := e2eDialogs(); dir != "" {
			return filepath.Join(dir, filepath.Base(o.DefaultFilename)), nil
		}
		return realSave(ctx, o)
	}
}

func e2eDialogs() string {
	if dir := os.Getenv("QUARTZ_E2E_DIALOGS"); filepath.IsAbs(dir) {
		return dir
	}
	return ""
}

// e2eAnswer reads one answer file; a missing one is a cancelled dialog.
func e2eAnswer(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test-only build, a folder the test chose
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
