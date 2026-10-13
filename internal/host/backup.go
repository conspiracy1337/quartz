package host

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"quartz/internal/backup"
	"quartz/internal/core/fsx"
)

// keepBackups is how many host backups of one world are kept.
const keepBackups = 5

// backupWorld zips one world into dir/<timestamp>.zip before a server plays
// it in place, then drops all but the newest keepBackups.
func backupWorld(world, dir string, now time.Time) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	out, err := fsx.SafeJoin(dir, now.Format("2006-01-02_15-04-05")+".zip")
	if err != nil {
		return err
	}
	if err := backup.ZipDir(world, out, filepath.Base(world)+"/"); err != nil {
		return err
	}
	return prune(dir, keepBackups)
}

// prune removes the oldest zips in dir beyond keep (names sort by time).
func prune(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var zips []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".zip") {
			zips = append(zips, e.Name())
		}
	}
	slices.Sort(zips)
	for len(zips) > keep {
		if err := os.Remove(filepath.Join(dir, zips[0])); err != nil {
			return err
		}
		zips = zips[1:]
	}
	return nil
}
