package modpack

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"sort"
	"strings"

	"quartz/internal/core/archive"
	"quartz/internal/core/hashx"
	"quartz/internal/modplatform"
)

// Deps are what reading packs needs: the platforms (CurseForge packs list
// file ids only), JSON APIs (FTB, Technic) and a way to fetch pack files.
type Deps struct {
	Modrinth   modplatform.Platform
	CurseForge modplatform.Platform
	HTTP       modplatform.Doer
	// Fetch downloads a pack archive to dest (https only; sum may be zero).
	Fetch func(ctx context.Context, rawURL string, size int64, sum hashx.Sum, dest string) error
	Cache string // where fetched packs go
	Log   *slog.Logger
}

// Reader reads packs. It is safe for concurrent use.
type Reader struct{ d Deps }

// NewReader returns a reader.
func NewReader(d Deps) *Reader {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Reader{d: d}
}

// File reads a pack zip on disk (any format but FTB, which has no zip).
func (r *Reader) File(ctx context.Context, file string) (*Pack, error) {
	zr, err := archive.Open(file)
	if err != nil {
		return nil, ErrNotPack
	}
	defer func() { _ = zr.Close() }()
	var p *Pack
	switch {
	case zr.Has("modrinth.index.json"):
		p, err = readMrpack(zr)
	case zr.Has("manifest.json"):
		p, err = r.readCurse(ctx, zr)
	case zr.Has("bin/modpack.jar") || zr.Has("bin/version.json"):
		p, err = readTechnic(zr, nil)
	default:
		p, err = readMMC(zr)
	}
	if err != nil {
		return nil, err
	}
	p.Archive = file
	if p.Icon == nil {
		for _, name := range []string{"icon.png", "overrides/icon.png"} {
			if b, err := zr.Read(name, maxIcon); err == nil {
				p.Icon = b
				break
			}
		}
	}
	return p, nil
}

// --- Modrinth .mrpack ---

// mrpackHosts are the only addresses an .mrpack may download from (the
// format's own rule).
var mrpackHosts = map[string]bool{"cdn.modrinth.com": true, "github.com": true, "raw.githubusercontent.com": true, "gitlab.com": true}

type mrIndex struct {
	FormatVersion int    `json:"formatVersion"`
	Game          string `json:"game"`
	VersionID     string `json:"versionId"`
	Name          string `json:"name"`
	Summary       string `json:"summary"`
	Files         []struct {
		Path   string            `json:"path"`
		Hashes map[string]string `json:"hashes"`
		Env    *struct {
			Client string `json:"client"`
		} `json:"env"`
		Downloads []string `json:"downloads"`
		FileSize  int64    `json:"fileSize"`
	} `json:"files"`
	Dependencies map[string]string `json:"dependencies"`
}

var mrLoaders = map[string]string{"fabric-loader": "fabric", "quilt-loader": "quilt", "forge": "forge", "neoforge": "neoforge"}

func readMrpack(zr *archive.Reader) (*Pack, error) {
	var ix mrIndex
	if err := readJSON(zr, "modrinth.index.json", &ix); err != nil {
		return nil, err
	}
	if ix.FormatVersion != 1 || ix.Game != "minecraft" {
		return nil, ErrNotPack
	}
	p := &Pack{Origin: Origin{Source: "file"}, Format: "Modrinth", Name: clip(ix.Name, 80), Version: clip(ix.VersionID, 40), Summary: clip(ix.Summary, 300),
		Layers: []string{"overrides/", "client-overrides/"}}
	for key, ver := range ix.Dependencies {
		switch {
		case key == "minecraft":
			p.Minecraft = ver
		case mrLoaders[key] != "":
			if p.Loader != "" {
				return nil, fmt.Errorf("modpack: the pack names two mod loaders")
			}
			p.Loader, p.LoaderVersion = mrLoaders[key], ver
		default:
			return nil, fmt.Errorf("modpack: the pack needs %q, which the launcher doesn't know", clip(key, 40))
		}
	}
	if p.Minecraft == "" {
		return nil, ErrNoMinecraft
	}
	if len(ix.Files) > maxFiles {
		return nil, fmt.Errorf("modpack: the pack lists too many files")
	}
	foreign := 0
	for _, f := range ix.Files {
		if f.Env != nil && f.Env.Client == "unsupported" {
			continue
		}
		rel, err := CleanPath(f.Path)
		if err != nil {
			return nil, err
		}
		sha1, sha512 := strings.ToLower(f.Hashes["sha1"]), strings.ToLower(f.Hashes["sha512"])
		if !hexOf(sha1, 40) || !hexOf(sha512, 128) {
			return nil, fmt.Errorf("modpack: %s has no checksum", path.Base(rel))
		}
		if f.FileSize < 0 || f.FileSize > maxFileSize {
			return nil, fmt.Errorf("modpack: %s is too large", path.Base(rel))
		}
		var urls []string
		for _, u := range f.Downloads {
			if allowedURL(u, mrpackHosts) {
				urls = append(urls, u)
			}
		}
		if len(urls) == 0 {
			foreign++
			continue
		}
		optional := f.Env != nil && f.Env.Client == "optional"
		if optional {
			rel += ".disabled"
		}
		p.Files = append(p.Files, File{Path: rel, SHA1: sha1, SHA512: sha512, Size: f.FileSize, URLs: urls, Optional: optional})
	}
	if foreign > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%d %s from addresses Modrinth packs may not use %s left out", foreign, plural(foreign, "file"), were(foreign)))
	}
	return p, nil
}

// --- CurseForge zip ---

type cfManifest struct {
	ManifestType    string `json:"manifestType"`
	ManifestVersion int    `json:"manifestVersion"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Minecraft       struct {
		Version    string `json:"version"`
		ModLoaders []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"modLoaders"`
	} `json:"minecraft"`
	Files []struct {
		ProjectID int64 `json:"projectID"`
		FileID    int64 `json:"fileID"`
		Required  *bool `json:"required"`
	} `json:"files"`
	Overrides string `json:"overrides"`
}

// cfLoader reads a CurseForge loader id: "forge-47.2.0", "neoforge-21.1.77",
// "neoforge-1.20.1-47.1.106", "fabric-0.16.9".
func cfLoader(id, mc string) (string, string, bool) {
	for _, kind := range []string{"neoforge", "forge", "fabric", "quilt"} {
		if v, ok := strings.CutPrefix(id, kind+"-"); ok && v != "" {
			v = strings.TrimPrefix(v, mc+"-")
			return kind, v, true
		}
	}
	return "", "", false
}

func (r *Reader) readCurse(ctx context.Context, zr *archive.Reader) (*Pack, error) {
	var m cfManifest
	if err := readJSON(zr, "manifest.json", &m); err != nil {
		return nil, err
	}
	if m.ManifestType != "minecraftModpack" || m.ManifestVersion != 1 {
		return nil, ErrNotPack
	}
	mc := strings.TrimRight(m.Minecraft.Version, ".")
	if mc == "" {
		return nil, ErrNoMinecraft
	}
	overrides := m.Overrides
	if overrides == "" {
		overrides = "overrides"
	}
	if o, err := CleanPath(overrides); err != nil || strings.Contains(o, "/") {
		return nil, ErrUnsafePath
	}
	p := &Pack{Origin: Origin{Source: "file"}, Format: "CurseForge", Name: clip(m.Name, 80), Version: clip(m.Version, 40), Minecraft: mc, Layers: []string{overrides + "/"}}
	for _, l := range m.Minecraft.ModLoaders {
		if kind, ver, ok := cfLoader(l.ID, mc); ok && (l.Primary || p.Loader == "") {
			p.Loader, p.LoaderVersion = kind, ver
		}
	}
	if len(m.Files) > maxFiles {
		return nil, fmt.Errorf("modpack: the pack lists too many files")
	}
	refs := make([]cfRef, 0, len(m.Files))
	for _, f := range m.Files {
		refs = append(refs, cfRef{project: f.ProjectID, file: f.FileID, optional: f.Required != nil && !*f.Required})
	}
	if err := r.resolveCurse(ctx, p, refs); err != nil {
		return nil, err
	}
	return p, nil
}

type cfRef struct {
	project, file int64
	optional      bool
}

// curseFolders is where each class of CurseForge file goes.
var curseFolders = map[modplatform.ProjectType]string{
	modplatform.TypeMod: "mods", modplatform.TypeResourcePack: "resourcepacks", modplatform.TypeShader: "shaderpacks",
}

// resolveCurse asks CurseForge for the files a pack lists by id.
func (r *Reader) resolveCurse(ctx context.Context, p *Pack, refs []cfRef) error {
	if len(refs) == 0 {
		return nil
	}
	cf := r.d.CurseForge
	if cf == nil || !cf.Ready() {
		return errors.New("modpack: CurseForge isn't set up in this build, so its packs can't be read")
	}
	ids := make([]string, 0, len(refs))
	for _, f := range refs {
		ids = append(ids, fmt.Sprint(f.file))
	}
	files, err := cf.Files(ctx, ids)
	if err != nil {
		return err
	}
	skipped, gone := 0, 0
	for _, ref := range refs {
		v, ok := files[fmt.Sprint(ref.file)]
		if !ok || v.ProjectID != fmt.Sprint(ref.project) {
			gone++ // removed by its author: the rest of the pack still installs (XMCL drops it too)
			continue
		}
		folder, known := curseFolders[v.Class]
		if !known {
			skipped++
			continue
		}
		name, err := CleanPath(v.File.Name)
		if err != nil || strings.Contains(name, "/") || !hexOf(v.File.SHA1, 40) {
			return fmt.Errorf("modpack: CurseForge file %d has no usable name or checksum", ref.file)
		}
		rel := folder + "/" + name
		if ref.optional {
			rel += ".disabled"
		}
		f := File{Path: rel, SHA1: v.File.SHA1, Size: v.File.Size, Name: v.Number, Page: v.File.Page,
			Project: modplatform.Key(modplatform.CurseForge, v.ProjectID), Version: v.Number, Optional: ref.optional}
		if v.File.URL != "" {
			if !modplatform.ValidURL(v.File.URL) {
				return fmt.Errorf("modpack: CurseForge gave an unexpected address for %s", name)
			}
			f.URLs = []string{v.File.URL}
		}
		p.Files = append(p.Files, f)
	}
	if skipped > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%d %s for worlds %s left out", skipped, plural(skipped, "file"), were(skipped)))
	}
	if gone > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%d %s CurseForge no longer has %s left out", gone, plural(gone, "file"), were(gone)))
	}
	r.modrinthCopies(ctx, p)
	return nil
}

// modrinthCopies gives blocked CurseForge files the download of the same
// file on Modrinth, found by sha1, so only the rest need a manual download
// (Prism does this by default). Modrinth being unreachable changes nothing.
func (r *Reader) modrinthCopies(ctx context.Context, p *Pack) {
	mr := r.d.Modrinth
	if mr == nil || !mr.Ready() {
		return
	}
	var ask []modplatform.Hashes
	for _, f := range p.Files {
		if f.Blocked() {
			ask = append(ask, modplatform.Hashes{SHA1: f.SHA1})
		}
	}
	if len(ask) == 0 {
		return
	}
	found, err := mr.Identify(ctx, ask)
	if err != nil {
		r.d.Log.Debug("modpack: Modrinth copies of blocked files", "err", err)
		return
	}
	for i, f := range p.Files {
		v, ok := found[f.SHA1]
		if !f.Blocked() || !ok || !strings.EqualFold(v.File.SHA1, f.SHA1) || !modplatform.ValidURL(v.File.URL) {
			continue
		}
		p.Files[i].URLs = []string{v.File.URL}
		if hexOf(strings.ToLower(v.File.SHA512), 128) {
			p.Files[i].SHA512 = strings.ToLower(v.File.SHA512)
		}
	}
}

// --- Prism and MultiMC instance zips ---

// knownComponents are the components an instance zip may have besides the
// game and its loader; anything else (jar mods, custom versions) is left out.
var knownComponents = map[string]bool{
	"org.lwjgl": true, "org.lwjgl3": true, "net.fabricmc.intermediary": true, "org.quiltmc.hashed": true,
}

var mmcLoaders = map[string]string{
	"net.fabricmc.fabric-loader": "fabric", "org.quiltmc.quilt-loader": "quilt", "net.minecraftforge": "forge", "net.neoforged": "neoforge",
}

// ReadMMCPack sets the game and loader from a Prism or MultiMC
// mmc-pack.json; components it can't install (jar mods, custom versions,
// LiteLoader) are left out with a warning.
func (p *Pack) ReadMMCPack(data []byte) error {
	var pack struct {
		Components []struct {
			UID     string `json:"uid"`
			Version string `json:"version"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &pack); err != nil {
		return ErrNotPack
	}
	unknown := 0
	for _, c := range pack.Components {
		switch {
		case c.UID == "net.minecraft":
			p.Minecraft = c.Version
		case mmcLoaders[c.UID] != "":
			p.Loader, p.LoaderVersion = mmcLoaders[c.UID], c.Version
		case !knownComponents[c.UID]:
			unknown++
		}
	}
	if p.Minecraft == "" {
		return ErrNoMinecraft
	}
	if unknown > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%d custom %s (such as jar mods) %s left out", unknown, plural(unknown, "component"), were(unknown)))
	}
	return nil
}

func readMMC(zr *archive.Reader) (*Pack, error) {
	cfgs := zr.Glob("", "instance.cfg")
	if len(cfgs) == 0 {
		return nil, ErrNotPack
	}
	sort.Slice(cfgs, func(a, b int) bool { return strings.Count(cfgs[a], "/") < strings.Count(cfgs[b], "/") })
	cfg := cfgs[0]
	if cfg != "instance.cfg" && !strings.HasSuffix(cfg, "/instance.cfg") {
		return nil, ErrNotPack
	}
	root := strings.TrimSuffix(cfg, "instance.cfg")
	if root != "" {
		if _, err := CleanPath(root + "x"); err != nil {
			return nil, ErrUnsafePath
		}
	}
	p := &Pack{Origin: Origin{Source: "file"}, Format: "Prism or MultiMC"}
	// Only the name is read: commands, JVM arguments and the environment an
	// instance file may carry are never taken over.
	if b, err := zr.Read(cfg, 1<<20); err == nil {
		sc := bufio.NewScanner(bytes.NewReader(b))
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "name="); ok {
				p.Name = clip(strings.TrimSpace(v), 80)
			}
		}
	}
	data, err := zr.Read(root+"mmc-pack.json", maxManifest)
	if err != nil {
		return nil, ErrNotPack
	}
	if err := p.ReadMMCPack(data); err != nil {
		return nil, err
	}
	for _, dir := range []string{".minecraft/", "minecraft/"} {
		if zr.HasPrefix(root + dir) {
			p.Layers = []string{root + dir}
			break
		}
	}
	return p, nil
}

// --- Technic ---

// readTechnic reads a Technic zip: the game folder at the root, the loader
// in bin/ (bin/version.json, or the version.json inside bin/modpack.jar, or
// the forgeversion.properties of very old packs). hint fills what the zip
// can't say (the Minecraft version, from Technic's API).
func readTechnic(zr *archive.Reader, hint *Pack) (*Pack, error) {
	p := &Pack{Origin: Origin{Source: "file"}, Format: "Technic", Layers: []string{""}, Skip: []string{"bin/"}}
	if hint != nil {
		p.Origin, p.Name, p.Version, p.Minecraft, p.IconURL = hint.Origin, hint.Name, hint.Version, hint.Minecraft, hint.IconURL
	}
	var version []byte
	if b, err := zr.Read("bin/version.json", maxManifest); err == nil {
		version = b
	} else if jar, err := zr.Read("bin/modpack.jar", 256<<20); err == nil {
		inner, err := archive.NewReader(bytes.NewReader(jar), int64(len(jar)))
		if err != nil {
			return nil, ErrNotPack
		}
		if b, err := inner.Read("version.json", maxManifest); err == nil {
			version = b
		} else if b, err := inner.Read("forgeversion.properties", 1<<20); err == nil {
			p.Loader, p.LoaderVersion = "forge", forgeProperties(b)
		}
	}
	if version != nil {
		mc, kind, ver := technicLoader(version)
		if p.Minecraft == "" {
			p.Minecraft = mc
		}
		if kind != "" {
			p.Loader, p.LoaderVersion = kind, ver
		}
	}
	if p.Minecraft == "" {
		return nil, ErrNoMinecraft
	}
	return p, nil
}

// technicLoader reads a version.json (a loader profile) the way Prism's
// TechnicPackProcessor does.
func technicLoader(b []byte) (mc, kind, ver string) {
	var v struct {
		ID           string `json:"id"`
		InheritsFrom string `json:"inheritsFrom"`
		Libraries    []struct {
			Name string `json:"name"`
		} `json:"libraries"`
		Arguments struct {
			Game []any `json:"game"`
		} `json:"arguments"`
	}
	if json.Unmarshal(b, &v) != nil {
		return "", "", ""
	}
	mc = v.InheritsFrom
	for _, l := range v.Libraries {
		switch {
		case strings.HasPrefix(l.Name, "net.neoforged:neoforge:"):
			kind, ver = "neoforge", strings.TrimPrefix(l.Name, "net.neoforged:neoforge:")
		case strings.HasPrefix(l.Name, "net.neoforged.fancymodloader:"):
			kind = "neoforge"
			for i, a := range v.Arguments.Game {
				if s, _ := a.(string); s == "--fml.neoForgeVersion" && i+1 < len(v.Arguments.Game) {
					ver, _ = v.Arguments.Game[i+1].(string)
				}
			}
		case strings.HasPrefix(l.Name, "net.minecraftforge:forge:") || strings.HasPrefix(l.Name, "net.minecraftforge:fmlloader:"):
			if kind != "neoforge" {
				parts := strings.Split(l.Name[strings.LastIndex(l.Name, ":")+1:], "-")
				if len(parts) >= 2 {
					if mc == "" {
						mc = parts[0]
					}
					kind, ver = "forge", parts[1]
				}
			}
		case strings.HasPrefix(l.Name, "net.fabricmc:fabric-loader:"):
			kind, ver = "fabric", strings.TrimPrefix(l.Name, "net.fabricmc:fabric-loader:")
		}
	}
	if mc == "" {
		mc, _, _ = strings.Cut(v.ID, "-")
	}
	return mc, kind, ver
}

func forgeProperties(b []byte) string {
	vals := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return strings.Join([]string{vals["forge.major.number"], vals["forge.minor.number"], vals["forge.revision.number"], vals["forge.build.number"]}, ".")
}

// --- helpers ---

func readJSON(zr *archive.Reader, name string, out any) error {
	b, err := zr.Read(name, maxManifest)
	if err != nil {
		return ErrNotPack
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("modpack: %s is damaged: %w", name, err)
	}
	return nil
}

// allowedURL reports whether raw is https on one of hosts (nil allows any host).
func allowedURL(raw string, hosts map[string]bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(raw) > 2048 {
		return false
	}
	return hosts == nil || hosts[strings.ToLower(u.Hostname())]
}

func hexOf(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func clip(s string, n int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s))
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

func plural(n int, one string) string {
	if n == 1 {
		return one
	}
	return one + "s"
}

func were(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}
