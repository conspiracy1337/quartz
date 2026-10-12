package api

import (
	"errors"
	"path/filepath"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/core/fsx"
	"quartz/internal/settings"
	"quartz/internal/theme"
)

// Settings exposes the global settings document and the custom CSS file.
type Settings struct {
	ctx   *Ctx
	store *settings.Store
	css   *theme.CustomCSS
}

func NewSettings(ctx *Ctx, store *settings.Store, css *theme.CustomCSS) *Settings {
	return &Settings{ctx: ctx, store: store, css: css}
}

func (s *Settings) Get() settings.Settings { return s.store.Get() }

// Save stores next after validation and returns what was stored. The custom CSS
// path can only be changed through PickCustomCSS, never set directly by the UI.
func (s *Settings) Save(next settings.Settings) settings.Settings {
	next.Appearance.CustomCSS.Path = s.store.Get().Appearance.CustomCSS.Path
	return s.store.Replace(next)
}

func (s *Settings) CustomCSS() theme.CSSState { return s.css.Current() }

// PickCustomCSS lets the user choose a CSS file and turns custom CSS on.
// It returns the stored settings; an empty pick changes nothing.
func (s *Settings) PickCustomCSS() (settings.Settings, error) {
	path, err := wruntime.OpenFileDialog(s.ctx.Get(), wruntime.OpenDialogOptions{
		Title:   "Choose a CSS file",
		Filters: []wruntime.FileFilter{{DisplayName: "CSS files (*.css)", Pattern: "*.css"}},
	})
	if err != nil || path == "" {
		return s.store.Get(), err
	}
	if err := theme.ValidatePath(path); err != nil {
		return s.store.Get(), err
	}
	next := s.store.Get()
	next.Appearance.CustomCSS = settings.CustomCSS{Enabled: true, Path: filepath.Clean(path)}
	return s.store.Replace(next), nil
}

// ErrThemeTooLarge is returned when an exported theme exceeds the CSS size cap.
var ErrThemeTooLarge = errors.New("api: theme is too large")

// ExportTheme saves css to a file the user picks.
func (s *Settings) ExportTheme(css string) error {
	if len(css) > theme.MaxCSSSize {
		return ErrThemeTooLarge
	}
	path, err := wruntime.SaveFileDialog(s.ctx.Get(), wruntime.SaveDialogOptions{
		Title:           "Export theme",
		DefaultFilename: "theme.css",
		Filters:         []wruntime.FileFilter{{DisplayName: "CSS files (*.css)", Pattern: "*.css"}},
	})
	if err != nil || path == "" {
		return err
	}
	if err := theme.ValidatePath(path); err != nil {
		return err
	}
	return fsx.AtomicWrite(path, []byte(css), fsx.FilePerm)
}
