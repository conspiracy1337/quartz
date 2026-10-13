//go:build windows

package shortcuts

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"

	"quartz/internal/brand"
	"quartz/internal/core/fsx"
	"quartz/internal/core/pngx"
)

// win writes .lnk files through the WScript.Shell COM object (no cgo).
type win struct{ desktopDir, menuDir string }

func newWriter() writer {
	desk, _ := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	progs, _ := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	w := win{desktopDir: desk}
	if progs != "" {
		w.menuDir = filepath.Join(progs, brand.Name)
	}
	return w
}

func (w win) dir(where Where) string {
	if where == Desktop {
		return w.desktopDir
	}
	return w.menuDir
}

func (w win) create(s Spec, iconDir string) (string, error) {
	dir := w.dir(s.Where)
	if dir == "" || !s.Where.Valid() {
		return "", ErrNoFolder
	}
	if s.Where == Menu {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", err
		}
	}
	ico, err := pngx.ICO(s.Icon)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(iconDir, 0o700); err != nil {
		return "", err
	}
	icon := filepath.Join(iconDir, s.ID+".ico")
	if err := fsx.AtomicWrite(icon, ico, fsx.FilePerm); err != nil {
		return "", err
	}
	path, err := free(dir, fsx.FileName(s.Name, s.ID), ".lnk")
	if err != nil {
		return "", err
	}
	args := make([]string, len(s.Args))
	for i, a := range s.Args {
		args[i] = syscall.EscapeArg(a)
	}
	return path, writeLnk(path, s.Exe, strings.Join(args, " "), filepath.Dir(s.Exe), icon, "Play "+s.Name)
}

// remove deletes a .lnk in our folders (a .lnk carries no marker we could
// read without COM; the recorded path and folder are the check).
func (w win) remove(path, _ string) error {
	if !strings.EqualFold(filepath.Ext(path), ".lnk") {
		return nil
	}
	d := filepath.Dir(path)
	if !strings.EqualFold(d, filepath.Clean(w.desktopDir)) && !strings.EqualFold(d, filepath.Clean(w.menuDir)) {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// writeLnk saves a shortcut through WScript.Shell.CreateShortcut.
func writeLnk(path, target, args, workdir, icon, desc string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		var oe *ole.OleError
		if !errors.As(err, &oe) || oe.Code() != 1 { // S_FALSE: already initialised
			return err
		}
	}
	defer ole.CoUninitialize()
	unk, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer unk.Release()
	shell, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	v, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		return err
	}
	lnk := v.ToIDispatch()
	defer lnk.Release()
	for _, p := range []struct{ name, value string }{
		{"TargetPath", target}, {"Arguments", args}, {"WorkingDirectory", workdir},
		{"IconLocation", icon + ",0"}, {"Description", desc},
	} {
		if _, err := oleutil.PutProperty(lnk, p.name, p.value); err != nil {
			return err
		}
	}
	_, err = oleutil.CallMethod(lnk, "Save")
	return err
}
