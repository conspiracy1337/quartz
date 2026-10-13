//go:build e2e

package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestE2EDialogsAnswerFromFolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUARTZ_E2E_DIALOGS", dir)
	got, err := saveFileDialog(context.Background(), wruntime.SaveDialogOptions{DefaultFilename: "../Pack.mrpack"})
	if err != nil || got != filepath.Join(dir, "Pack.mrpack") {
		t.Fatalf("save = %q, %v", got, err)
	}
	if got, _ := openFileDialog(context.Background(), wruntime.OpenDialogOptions{}); got != "" {
		t.Fatalf("no answer must read as cancelled, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "open"), []byte("/tmp/x.mrpack\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := openFileDialog(context.Background(), wruntime.OpenDialogOptions{}); got != "/tmp/x.mrpack" {
		t.Fatalf("open = %q", got)
	}
}
