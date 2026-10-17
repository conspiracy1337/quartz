package worlds

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/core/hashx"
	"quartz/internal/core/netx"
	"quartz/internal/core/tasks"
	"quartz/internal/modplatform"
)

// maxWorldFile bounds a world download (big adventure maps run to a few GB).
const maxWorldFile = 8 << 30

// Errors from Install.
var (
	ErrWebsiteOnly = errors.New("its author only allows downloads from the CurseForge website")
	ErrBadFile     = errors.New("the platform gave no usable download for it")
)

// Install downloads a world a platform offers (a .zip) as a job on the
// Downloads page and adds it to the instance once it has arrived, as Import
// does. It returns the job's id.
func (s *Service) Install(id, title string, f modplatform.File) (string, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return "", err
	}
	switch {
	case f.URL == "":
		return "", ErrWebsiteOnly
	case !modplatform.ValidURL(f.URL), !store.ValidSHA1(f.SHA1), f.Size <= 0, f.Size > maxWorldFile:
		return "", ErrBadFile
	}
	if err := os.MkdirAll(s.d.Downloads, 0o700); err != nil {
		return "", err
	}
	dest := filepath.Join(s.d.Downloads, random()+".zip")
	spec := tasks.NewJob(tasks.Meta{Title: title, Subtitle: in.Name, Art: in.TaskArt(), Kind: "install"}).
		Step("Download", tasks.Bytes, tasks.Download(download.Item{URLs: []string{f.URL}, Dest: dest, Size: f.Size, Sum: hashx.Of(hashx.SHA1, f.SHA1)})).
		After("Adding the world", tasks.Count, tasks.Func("add "+title, func(context.Context) error {
			defer func() { _ = os.Remove(dest) }()
			if _, err := s.Import(id, dest); err != nil {
				return &netx.PermanentError{Err: err}
			}
			return nil
		}))
	return s.d.Tasks.Submit(spec)
}
