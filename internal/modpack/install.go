package modpack

import (
	"context"
	"encoding/json"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
	"quartz/internal/core/netx"
	"quartz/internal/core/tasks"
	"quartz/internal/icons"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/loader"
	"quartz/internal/manual"
)

// A pack read for the Import screen waits this long for Install.
const stagedLife = 30 * time.Minute

// ErrExpired is returned for an unknown or expired import token.
var ErrExpired = errors.New("modpack: this import is too old; open the pack again")

// InstallDeps wire installs and updates.
type InstallDeps struct {
	Reader    *Reader
	Instances *instance.Store
	Store     *store.Store
	Tasks     *tasks.Engine
	Manual    *manual.Service
	Loaders   *loader.Registry
	Icons     *icons.Library
	// Prepare plans the game files of a target (launch.Prepare); a nil plan
	// means everything is in place.
	Prepare func(ctx context.Context, t install.Target, gameDir string, meta tasks.Meta) (*tasks.Spec, error)
	// Defaults are the global Java settings new instances start from.
	Defaults func() instance.JavaSettings
	// Sandboxed says whether new instances start sandboxed (when this
	// system can sandbox games; nil: never).
	Sandboxed func() bool
	// GetIcon fetches a pack's icon from its platform CDN.
	GetIcon func(ctx context.Context, rawURL string) ([]byte, error)
	Staging string // instances/.staging
	Bus     *events.Bus
	Log     *slog.Logger
	// Recognise looks for the pack of an imported instance in the
	// background once it is installed (see Recognise).
	Recognise bool
	// Index scans an instance's content once a pack put or changed files in
	// it, so its counts and lists are right before anyone opens a tab.
	Index func(ctx context.Context, id string)
}

// Service installs and updates packs. It is safe for concurrent use.
type Service struct {
	d InstallDeps
	u UpdateDeps

	mu        sync.Mutex
	staged    map[string]staged
	available map[string]Available // newer versions, by instance
	pending   map[string]pending   // newer versions read for review, by instance
}

type staged struct {
	pack *Pack
	at   time.Time
}

// NewService returns a pack service.
func NewService(d InstallDeps, u UpdateDeps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d, u: u, staged: map[string]staged{}, available: map[string]Available{}, pending: map[string]pending{}}
}

var tokenRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Stage keeps a read pack for Install and returns its token.
func (s *Service) Stage(p *Pack) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	tok := hex.EncodeToString(b[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.staged {
		if time.Since(v.at) > stagedLife {
			delete(s.staged, k)
		}
	}
	s.staged[tok] = staged{pack: p, at: time.Now()}
	return tok
}

// Staged returns a staged pack.
func (s *Service) Staged(token string) (*Pack, error) {
	if !tokenRe.MatchString(token) {
		return nil, ErrExpired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.staged[token]
	if !ok || time.Since(st.at) > stagedLife {
		return nil, ErrExpired
	}
	return st.pack, nil
}

// take removes a staged pack (an import is single use).
func (s *Service) take(token string) (*Pack, error) {
	p, err := s.Staged(token)
	if err == nil {
		s.mu.Lock()
		delete(s.staged, token)
		s.mu.Unlock()
	}
	return p, err
}

// Install creates an instance from a staged pack as one job: the game and
// loader, the pack's files (the optional ones in enable turned on), any
// manual downloads, then the instance itself.
// The instance appears only when everything is in place; a failed or
// cancelled job leaves nothing behind.
func (s *Service) Install(ctx context.Context, token, name string, art instance.Art, enable []string) (string, error) {
	p, err := s.take(token)
	if err != nil {
		return "", err
	}
	p.Enable(enable)
	target, err := s.target(ctx, p)
	if err != nil {
		return "", err
	}
	work := filepath.Join(s.d.Staging, newToken())
	game := filepath.Join(work, "game")
	if err := os.MkdirAll(game, 0o700); err != nil {
		return "", err
	}
	meta := tasks.Meta{Title: orElse(name, p.Name), Subtitle: "Installing as a new instance", Art: tasks.Art{Background: art.Background, Initial: art.Initial}, Kind: "install"}
	files, set, packSpec, err := s.filesSpec(meta, p, work)
	if err != nil {
		_ = os.RemoveAll(work)
		return "", err
	}
	packSpec.After("Installing", tasks.Count, tasks.Func("create the instance", func(ctx context.Context) error {
		err := s.create(ctx, p, files, set, target, game, instance.CreateRequest{Name: orElse(name, p.Name), Art: art, Sandbox: s.d.Sandboxed != nil && s.d.Sandboxed()})
		if err != nil {
			return &netx.PermanentError{Err: err}
		}
		return nil
	}))
	spec := packSpec
	if s.d.Prepare != nil && target.Minecraft != "" {
		g, err := s.d.Prepare(ctx, target, game, meta)
		if err != nil {
			_ = os.RemoveAll(work)
			return "", err
		}
		if g != nil {
			spec = g.Append(packSpec)
		}
	}
	id, err := s.d.Tasks.Submit(spec)
	if set != nil {
		set.Bind(id)
	}
	if err != nil {
		_ = os.RemoveAll(work)
		return "", err
	}
	// The job outlives the request that started it.
	go func() { //nolint:gosec // G118: deliberately not tied to ctx
		_ = s.d.Tasks.Wait(context.Background(), id)
		_ = os.RemoveAll(work) // the game folder moved out on success
	}()
	return id, nil
}

// target resolves the pack's loader build in the registry.
func (s *Service) target(ctx context.Context, p *Pack) (install.Target, error) {
	t := install.Target{Minecraft: p.Minecraft}
	if p.Loader == "" {
		return t, nil
	}
	k, err := loader.ParseKind(p.Loader)
	if err != nil {
		return t, err
	}
	t.Loader = k
	var v loader.Version
	if p.LoaderVersion == "" {
		v, err = s.d.Loaders.Latest(ctx, k, p.Minecraft)
	} else {
		v, err = s.d.Loaders.Match(ctx, k, p.Minecraft, p.LoaderVersion)
	}
	if err != nil {
		return t, err
	}
	t.LoaderVersion = v.ID
	return t, nil
}

// planned is one pack file on its way into the instance.
type planned struct {
	File
	dest string // where the download lands (a store blob, or work/ for extracted zips)
}

// filesSpec plans the downloads: into the store by sha1 when the pack
// names one, else into the work folder to be ingested.
func (s *Service) filesSpec(meta tasks.Meta, p *Pack, work string) ([]planned, *manual.Set, *tasks.Spec, error) {
	var files []planned
	var downloads []tasks.Work
	var needs []manual.Need
	for i, f := range p.Files {
		pf := planned{File: f}
		switch {
		case f.Blocked():
			needs = append(needs, manual.Need{Name: orElse(f.Name, path.Base(f.Path)), FileName: path.Base(strings.TrimSuffix(f.Path, ".disabled")),
				Size: f.Size, SHA1: f.SHA1, Page: f.Page})
			pf.dest = s.d.Store.Path(f.SHA1)
		case f.SHA1 != "" && !f.Extract:
			pf.dest = s.d.Store.Path(f.SHA1)
		default:
			pf.dest = filepath.Join(work, "files", fmt.Sprintf("%d", i))
		}
		if !f.Blocked() {
			it := download.Item{URLs: f.URLs, Dest: pf.dest, Size: f.Size}
			switch {
			case f.SHA512 != "":
				it.Sum = hashx.Of(hashx.SHA512, f.SHA512)
			case f.SHA1 != "":
				it.Sum = hashx.Of(hashx.SHA1, f.SHA1)
			case f.MD5 != "":
				it.Sum = hashx.Of(hashx.MD5, f.MD5)
			default:
				return nil, nil, nil, fmt.Errorf("modpack: %s has no checksum", path.Base(f.Path))
			}
			if err := os.MkdirAll(filepath.Dir(pf.dest), 0o700); err != nil {
				return nil, nil, nil, err
			}
			downloads = append(downloads, tasks.Download(it))
		}
		files = append(files, pf)
	}
	spec := tasks.NewJob(meta)
	if len(downloads) > 0 {
		spec.Step("Mods", tasks.Bytes, downloads...)
	}
	var set *manual.Set
	if len(needs) > 0 {
		if s.d.Manual == nil {
			return nil, nil, nil, errors.New("modpack: some files can only be downloaded from the CurseForge website")
		}
		set = s.d.Manual.Register(needs)
		spec.Step(manual.Label(len(needs), "mod"), tasks.Count, set.Work())
	}
	return files, set, spec, nil
}

// create puts the downloaded pack into a new instance.
func (s *Service) create(ctx context.Context, p *Pack, files []planned, set *manual.Set, t install.Target, game string, req instance.CreateRequest) error {
	root, err := os.OpenRoot(game)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	m := Manifest{Origin: p.Origin, Name: p.Name, Version: p.Version, Files: map[string]ManifestFile{}}
	layers, err := p.Extract(root)
	if err != nil {
		return err
	}
	for rel, sum := range layers {
		m.Files[rel] = ManifestFile{SHA1: sum, Layer: true}
	}
	for _, f := range files {
		if f.Blocked() && set != nil && set.Skipped(f.SHA1) {
			continue
		}
		if f.Extract {
			got := map[string]string{}
			if err := extractZip(f.dest, []string{""}, &Pack{Skip: []string{"bin/"}}, root, got); err != nil {
				return err
			}
			if t.Loader == "" {
				t = s.detectLoader(f.dest, t)
			}
			continue
		}
		sum, err := s.blob(f)
		if err != nil {
			return err
		}
		if dir := path.Dir(f.Path); dir != "." {
			if err := root.MkdirAll(dir, 0o700); err != nil {
				return err
			}
		}
		if _, err := s.d.Store.Place(sum, filepath.Join(game, filepath.FromSlash(f.Path))); err != nil {
			return err
		}
		m.Files[strings.TrimSuffix(f.Path, ".disabled")] = ManifestFile{SHA1: sum, Off: strings.HasSuffix(f.Path, ".disabled")}
	}
	_ = root.Close() // Windows can't rename a folder that is open
	req.Minecraft, req.Loader, req.LoaderVersion = t.Minecraft, string(t.Loader), t.LoaderVersion
	req.Icon = s.icon(ctx, p)
	defaults := instance.JavaSettings{}
	if s.d.Defaults != nil {
		defaults = s.d.Defaults()
	}
	in, err := s.d.Instances.Create(req)
	if err != nil {
		return err
	}
	if p.Folder != nil && p.Folder.InPlace {
		// The game folder stays the other launcher's: no marker is written,
		// so purging the instance never deletes it.
		_, err := s.d.Instances.Update(in.ID, func(i *instance.Instance) error {
			i.Location, i.InPlace = p.Folder.Path, true
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		dest := s.d.Instances.GameDir(in.ID)
		if err := os.Remove(dest); err == nil || errors.Is(err, os.ErrNotExist) {
			if err := os.Rename(game, dest); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if p.Imported != nil {
		if err := s.imported(in.ID, p.Imported, defaults); err != nil {
			return err
		}
	}
	inPlace := p.Folder != nil && p.Folder.InPlace
	switch {
	case p.Managed.Valid() && !inPlace:
		// The other launcher recorded the pack: linked now, its manifest
		// built from that version at the first update review.
		if err := s.link(in.ID, *p.Managed); err != nil {
			return err
		}
		s.checkLater(in.ID)
	case p.Imported != nil:
		// Another launcher's instance is no pack to update, unless Recognise finds it.
	default:
		if _, err := s.d.Instances.Update(in.ID, func(i *instance.Instance) error {
			i.Pack = &instance.PackRef{Source: p.Origin.Source, Project: p.Origin.Project, Version: p.Origin.Version, Name: p.Name, VersionName: p.Version}
			return nil
		}); err != nil {
			return err
		}
		// An instance zip's files are whatever its player made of them, not
		// a pack version's own: no manifest is claimed for them.
		if !p.Copied {
			if err := writeManifest(s.d.Instances.Dir(in.ID), m); err != nil {
				return err
			}
		}
	}
	if s.d.Index != nil {
		s.d.Index(ctx, in.ID)
	}
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.InstancesChanged, nil)
	}
	if p.Copied && !inPlace && !p.Managed.Valid() && s.d.Recognise {
		s.recogniseLater(in.ID, p.Name)
	}
	return nil
}

// link records the pack an instance came from.
func (s *Service) link(id string, m Managed) error {
	_, err := s.d.Instances.Update(id, func(i *instance.Instance) error {
		i.Pack = &instance.PackRef{Source: m.Origin.Source, Project: m.Origin.Project, Version: m.Origin.Version, Name: m.Name, VersionName: m.VersionName}
		return nil
	})
	return err
}

// imported records what an instance brought from another launcher: its
// playtime, its memory and where it came from.
func (s *Service) imported(id string, im *Imported, defaults instance.JavaSettings) error {
	_, err := s.d.Instances.Update(id, func(i *instance.Instance) error {
		i.PlaytimeMin = max(im.PlaytimeMin, 0)
		i.LastPlayed = im.LastPlayed
		i.ImportedFrom = &instance.ImportedFrom{Launcher: im.Launcher, Path: im.Path}
		if im.MemoryMB > 0 {
			j := defaults
			j.Override = true
			j.MemoryMaxMB = im.MemoryMB
			j.MemoryMinMB = min(j.MemoryMinMB, im.MemoryMB)
			i.Settings.Java = j
		}
		return nil
	})
	return err
}

// blob makes sure a downloaded file is in the store and returns its sha1.
func (s *Service) blob(f planned) (string, error) {
	if f.SHA1 != "" && f.dest == s.d.Store.Path(f.SHA1) {
		if _, ok := s.d.Store.Blob(f.SHA1); ok {
			return f.SHA1, nil
		}
		h, err := s.d.Store.Seal(f.SHA1)
		return h.SHA1, err
	}
	h, err := s.d.Store.Ingest(f.dest)
	return h.SHA1, err
}

// detectLoader reads a Technic Solder zip's bin/ for the loader.
func (s *Service) detectLoader(zip string, t install.Target) install.Target {
	p, err := s.d.Reader.File(context.Background(), zip)
	if err != nil || p.Loader == "" {
		return t
	}
	k, err := loader.ParseKind(p.Loader)
	if err != nil {
		return t
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if v, err := s.d.Loaders.Match(ctx, k, t.Minecraft, p.LoaderVersion); err == nil {
		t.Loader, t.LoaderVersion = k, v.ID
	}
	return t
}

// icon stores the pack's icon, from the zip or its platform ("" for none).
func (s *Service) icon(ctx context.Context, p *Pack) string {
	data := p.Icon
	if data == nil && p.IconURL != "" && s.d.GetIcon != nil {
		data, _ = s.d.GetIcon(ctx, p.IconURL)
	}
	if data == nil || s.d.Icons == nil {
		return ""
	}
	hash, err := s.d.Icons.Add(data)
	if err != nil {
		s.d.Log.Debug("modpack: icon not usable", "err", err)
		return ""
	}
	return hash
}

// GetIcon is an InstallDeps.GetIcon over an HTTP client.
func GetIcon(c *netx.Client) func(ctx context.Context, rawURL string) ([]byte, error) {
	return func(ctx context.Context, rawURL string) ([]byte, error) {
		if !allowedURL(rawURL, nil) {
			return nil, errors.New("modpack: bad icon address")
		}
		resp, err := c.Get(ctx, rawURL, http.Header{})
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxIcon+1))
		if err != nil || len(b) > maxIcon {
			return nil, errors.New("modpack: icon too large")
		}
		return b, nil
	}
}

// Manifest is instances/<id>/modpack.json: what the pack put where, so an
// update can tell the player's edits from the pack's files.
type Manifest struct {
	Origin  Origin                  `json:"origin"`
	Name    string                  `json:"name"`
	Version string                  `json:"version"`
	Files   map[string]ManifestFile `json:"files"` // by path, without .disabled
}

// ManifestFile is one file a pack installed.
type ManifestFile struct {
	SHA1  string `json:"sha1"`
	Layer bool   `json:"layer,omitempty"` // from the pack's overrides, not a download
	Off   bool   `json:"off,omitempty"`   // the pack ships it turned off
}

const manifestName = "modpack.json"

func writeManifest(dir string, m Manifest) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(filepath.Join(dir, manifestName), b, fsx.FilePerm)
}

// ReadManifest reads an instance's pack manifest.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	b, err := fsx.ReadCapped(filepath.Join(dir, manifestName), maxManifest)
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if m.Files == nil {
		m.Files = map[string]ManifestFile{}
	}
	return m, err
}

func newToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
