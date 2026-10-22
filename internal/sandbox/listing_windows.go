package sandbox

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"

	"quartz/internal/core/procx"
)

// Java 24 and later load their security settings (java.security) through
// Path.toRealPath(), which looks every folder of java.home up in its parent
// (FindFirstFile), from the drive root down. Passing through a folder isn't
// enough for that: the parent has to be listable, or every game on Java 25
// dies at startup with "Error loading java.security file". So the Java
// runtime's parent folders let the store capability list them: their names
// only, on each folder alone, never what is in their files. The launcher
// grants the folders the player owns; the others (a drive root, C:\Users)
// take one administrator approval, which stays for good. An older Java
// doesn't walk the path, so it never asks.

// accessList is FILE_LIST_DIRECTORY | FILE_READ_ATTRIBUTES | FILE_TRAVERSE |
// SYNCHRONIZE: what FindFirstFile needs on a parent.
const accessList = 0x001000A1

// listArg starts the launcher as the administrator helper that grants it.
const listArg = "--sandbox-allow-listing"

// maxListed bounds the folders one approval covers (a path's depth).
const maxListed = 64

// allowJavaListing makes java's parent folders listable to the store
// capability, asking for approval once where the player can't change a
// folder's permissions.
func (s *Service) allowJavaListing(java string, store *windows.SID) error {
	if java == "" {
		return nil
	}
	var denied []string
	for p := filepath.Dir(java); ; p = filepath.Dir(p) {
		if err := grantPath(p, store, accessList, false); err != nil {
			if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				return fmt.Errorf("can't let the game see the folder %s: %w", p, err)
			}
			denied = append(denied, p)
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	if len(denied) == 0 || !walksJavaHome(java) {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	s.c.Log.Info("sandbox: asking once to let games list the folders above Java", "folders", denied)
	code, err := procx.RunElevated(exe, append([]string{listArg}, denied...))
	if errors.Is(err, procx.ErrDeclined) {
		return ErrApproval
	}
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("letting the game see the folders above Java failed (exit code %d)", code)
	}
	return nil
}

// walksJavaHome reports whether the runtime is Java 24 or later, read from
// its release file; one that can't be read is assumed to be new.
func walksJavaHome(java string) bool {
	f, err := os.Open(filepath.Join(java, "release")) //nolint:gosec // the runtime the launcher picked
	if err != nil {
		return true
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		v, ok := strings.CutPrefix(sc.Text(), "JAVA_VERSION=")
		if !ok {
			continue
		}
		v = strings.TrimPrefix(strings.Trim(v, `"`), "1.") // 1.8.0_402 is Java 8
		major, _, _ := strings.Cut(v, ".")
		n, err := strconv.Atoi(major)
		return err != nil || n >= 24
	}
	return true
}

// Main runs the administrator helper when the launcher was started as one
// (listArg, from allowJavaListing) and returns at once otherwise. The helper
// can only make the given folders listable to the store capability, each
// folder alone: it takes no capability, access or inheritance from outside.
func Main() {
	if len(os.Args) < 2 || os.Args[1] != listArg {
		return
	}
	if err := allowListing(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "quartz: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// allowListing grants accessList on each folder: a clean absolute path on a
// drive, that exists and is a folder.
func allowListing(folders []string) error {
	if len(folders) == 0 || len(folders) > maxListed {
		return fmt.Errorf("sandbox: %d folders to list", len(folders))
	}
	store, err := capabilitySID(storeCapability)
	if err != nil {
		return err
	}
	for _, p := range folders {
		if len(filepath.VolumeName(p)) != 2 || !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return fmt.Errorf("sandbox: %q isn't a folder on a drive", p)
		}
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			return fmt.Errorf("sandbox: %q isn't a folder", p)
		}
		if err := grantPath(p, store, accessList, false); err != nil {
			return fmt.Errorf("sandbox: %s: %w", p, err)
		}
	}
	return nil
}
