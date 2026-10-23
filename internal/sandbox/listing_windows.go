package sandbox

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"

	"quartz/internal/core/procx"
)

// Path.toRealPath() looks every folder of a path up in its parent
// (FindFirstFile), from the drive root down, so passing through a folder
// isn't enough for it: the parent has to be listable. Java 24 and later load
// their security settings (java.security) that way, or every game on Java 25
// dies at startup with "Error loading java.security file", and mod loaders
// resolve the game folder that way on any Java (Fabric's mods folder). So the
// parent folders of the game folder and of the Java runtime let the store
// capability list them: their names only (a sibling instance's folder name),
// on each folder alone, never what is in their files. The launcher grants the
// folders the player owns; the others (a drive root, C:\Users) take one
// administrator approval, which stays for good.

// accessList is FILE_LIST_DIRECTORY | FILE_READ_ATTRIBUTES | FILE_TRAVERSE |
// SYNCHRONIZE: what FindFirstFile needs on a parent.
const accessList = 0x001000A1

// listArg starts the launcher as the administrator helper that grants it.
const listArg = "--sandbox-allow-listing"

// maxListed bounds the folders one approval covers (a path's depth).
const maxListed = 64

// allowParentListing makes the parent folders of the game folder and of Java
// listable to the store capability. The folders the player can't change wait
// for ApproveListing: the run fails with ErrApproval, so the launcher can say
// why before Windows asks. Java's parents need it only on Java 24 and later,
// which walk java.home.
func (s *Service) allowParentListing(r Run, store *windows.SID) error {
	newJava := r.Java != "" && walksJavaHome(r.Java)
	var denied []string
	for _, root := range []string{r.GameDir, r.Java} {
		if root == "" {
			continue
		}
		d, err := grantParents(root, store)
		if err != nil {
			return err
		}
		if root == r.GameDir || newJava {
			denied = append(denied, d...)
		}
	}
	if len(denied) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range denied {
		if !slices.Contains(s.pending, p) {
			s.pending = append(s.pending, p)
		}
	}
	return ErrApproval
}

// grantParents makes the folders above dir listable and returns the ones the
// player can't change.
func grantParents(dir string, store *windows.SID) ([]string, error) {
	var denied []string
	for p := filepath.Dir(dir); ; p = filepath.Dir(p) {
		if err := grantPath(p, store, accessList, false); err != nil {
			if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				return nil, fmt.Errorf("can't let the game see the folder %s: %w", p, err)
			}
			denied = append(denied, p)
		}
		if p == filepath.Dir(p) {
			return denied, nil
		}
	}
}

// ApproveListing asks Windows once (its UAC prompt) to let games list the
// folders a run waited for, and every folder above the data folder with
// them, so no instance on that drive asks again.
func (s *Service) ApproveListing() error {
	store, err := capabilitySID(storeCapability)
	if err != nil {
		return err
	}
	folders, err := grantParents(filepath.Join(s.c.DataRoot, "instances"), store)
	if err != nil {
		return err
	}
	s.mu.Lock()
	for _, p := range s.pending {
		if !slices.Contains(folders, p) {
			folders = append(folders, p)
		}
	}
	s.mu.Unlock()
	if len(folders) == 0 {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	s.c.Log.Info("sandbox: asking once to let games list the folders above the game and Java", "folders", folders)
	code, err := procx.RunElevated(exe, append([]string{listArg}, folders...))
	if errors.Is(err, procx.ErrDeclined) {
		return ErrDeclined
	}
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("letting games see the folders above them failed (exit code %d)", code)
	}
	s.mu.Lock()
	s.pending = slices.DeleteFunc(s.pending, func(p string) bool { return slices.Contains(folders, p) })
	s.mu.Unlock()
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
// (listArg, from ApproveListing) and returns at once otherwise. The helper
// can only make the given folders listable to the store capability, each
// folder alone: it takes no capability, access or inheritance from outside.
func Main() {
	if len(os.Args) < 2 || os.Args[1] != listArg {
		return
	}
	if err := grantListing(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "quartz: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// grantListing grants accessList on each folder: a clean absolute path on a
// drive, that exists and is a folder.
func grantListing(folders []string) error {
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
