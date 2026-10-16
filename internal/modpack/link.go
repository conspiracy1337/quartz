package modpack

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"quartz/internal/core/hashx"
	"quartz/internal/core/netx"
	"quartz/internal/modplatform"
)

// ErrLink is returned for a link that names no modpack.
var ErrLink = errors.New("modpack: this link isn't a modpack the launcher can read")

// maxPackFile caps a pack archive fetched from a link.
const maxPackFile = 1 << 30

var (
	ftbPath     = regexp.MustCompile(`^/modpacks?/(\d{1,9})(?:-[a-z0-9-]*)?/?$`)
	technicPath = regexp.MustCompile(`^/modpack/([a-z0-9-]{1,128})(?:\.\d+)?/?$`)
)

// Link reads the pack a pasted link names: a Modrinth or CurseForge pack
// page (optionally one version), an FTB or Technic pack page, or a direct
// https link to a pack file.
func (r *Reader) Link(ctx context.Context, raw string) (*Pack, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" || len(raw) > 2048 {
		return nil, ErrLink
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch host {
	case "modrinth.com":
		// /modpack/<slug>[/version/<id or number>]
		if len(segs) < 2 || (segs[0] != "modpack" && segs[0] != "project") {
			return nil, ErrLink
		}
		version := ""
		if len(segs) >= 4 && segs[2] == "version" {
			version = segs[3]
		}
		return r.modrinth(ctx, segs[1], version, true)
	case "curseforge.com":
		// /minecraft/modpacks/<slug>[/files/<id>]
		if len(segs) < 3 || segs[0] != "minecraft" || segs[1] != "modpacks" {
			return nil, ErrLink
		}
		file := ""
		if len(segs) >= 5 && (segs[3] == "files" || segs[3] == "download") {
			file = segs[4]
		}
		return r.curse(ctx, segs[2], "", file, true)
	case "feed-the-beast.com":
		if m := ftbPath.FindStringSubmatch(u.Path); m != nil {
			return r.ftb(ctx, m[1], "")
		}
		return nil, ErrLink
	case "technicpack.net":
		if m := technicPath.FindStringSubmatch(u.Path); m != nil {
			return r.technic(ctx, m[1])
		}
		return nil, ErrLink
	}
	// A direct link to a pack file, from any web page: never a host on this
	// computer or its network.
	dest, err := r.fetch(netx.PublicOnly(ctx), u.String(), 0, hashx.Sum{})
	if err != nil {
		return nil, err
	}
	return r.File(ctx, dest)
}

// Project reads a pack from a Modrinth or CurseForge project, whole.
func (r *Reader) Project(ctx context.Context, src modplatform.Source, id, version string) (*Pack, error) {
	return r.project(ctx, src, id, version, false)
}

// Preview reads a pack for the Import screen: a large one in place, so it
// shows at once; Complete makes it installable.
func (r *Reader) Preview(ctx context.Context, src modplatform.Source, id, version string) (*Pack, error) {
	return r.project(ctx, src, id, version, true)
}

func (r *Reader) project(ctx context.Context, src modplatform.Source, id, version string, peek bool) (*Pack, error) {
	if src == modplatform.CurseForge {
		return r.curse(ctx, "", id, version, peek)
	}
	return r.modrinth(ctx, id, version, peek)
}

// modrinth reads a Modrinth pack: the version asked for (by id or number)
// or the newest one, whose .mrpack is fetched and read.
func (r *Reader) modrinth(ctx context.Context, project, version string, peek bool) (*Pack, error) {
	mr := r.d.Modrinth
	if mr == nil {
		return nil, ErrLink
	}
	d, err := mr.Project(ctx, project)
	if err != nil || d.Type != modplatform.TypeModpack {
		return nil, ErrLink
	}
	vs, err := mr.Versions(ctx, d.ID, modplatform.Query{})
	if err != nil {
		return nil, err
	}
	v, ok := pickVersion(vs, version)
	if !ok {
		return nil, fmt.Errorf("modpack: %s has no such version", d.Title)
	}
	if !modplatform.ValidURL(v.File.URL) || !strings.HasSuffix(v.File.Name, ".mrpack") {
		return nil, ErrLink
	}
	sum := hashx.Of(hashx.SHA1, v.File.SHA1)
	if len(v.File.SHA512) == 128 {
		sum = hashx.Of(hashx.SHA512, v.File.SHA512)
	}
	p, err := r.open(ctx, v.File.URL, v.File.Size, sum, peek)
	if err != nil {
		return nil, err
	}
	p.Origin = Origin{Source: "modrinth", Project: d.ID, Version: v.ID, Slug: d.Slug}
	p.Name, p.Summary, p.IconURL = orElse(d.Title, p.Name), orElse(d.Summary, p.Summary), d.Icon
	p.Version = orElse(v.Number, p.Version)
	return p, nil
}

// curse reads a CurseForge pack by slug or id: the file asked for, or the newest.
func (r *Reader) curse(ctx context.Context, slug, id, file string, peek bool) (*Pack, error) {
	cf := r.d.CurseForge
	if cf == nil || !cf.Ready() {
		return nil, errors.New("modpack: CurseForge isn't set up in this build, so its packs can't be read")
	}
	if id == "" {
		var err error
		if id, err = cf.Slug(ctx, modplatform.TypeModpack, slug); err != nil {
			return nil, ErrLink
		}
	}
	d, err := cf.Project(ctx, id)
	if err != nil || d.Type != modplatform.TypeModpack {
		return nil, ErrLink
	}
	var v modplatform.Version
	if file != "" {
		if v, err = cf.Version(ctx, id, file); err != nil {
			return nil, err
		}
	} else {
		vs, err := cf.Versions(ctx, id, modplatform.Query{})
		if err != nil {
			return nil, err
		}
		var ok bool
		if v, ok = pickVersion(vs, ""); !ok {
			return nil, fmt.Errorf("modpack: %s has no files", d.Title)
		}
	}
	if v.File.URL == "" {
		return nil, ErrBlockedPack
	}
	if !modplatform.ValidURL(v.File.URL) {
		return nil, ErrLink
	}
	var sum hashx.Sum
	if hexOf(v.File.SHA1, 40) {
		sum = hashx.Of(hashx.SHA1, v.File.SHA1)
	}
	p, err := r.open(ctx, v.File.URL, v.File.Size, sum, peek)
	if err != nil {
		return nil, err
	}
	p.Origin = Origin{Source: "curseforge", Project: d.ID, Version: v.ID, Slug: d.Slug}
	p.Name, p.Summary, p.IconURL = orElse(d.Title, p.Name), d.Summary, d.Icon
	p.Version = orElse(p.Version, v.Number)
	return p, nil
}

// pickVersion is the version asked for (id or number), else the newest
// release, else the newest of any kind.
func pickVersion(vs []modplatform.Version, want string) (modplatform.Version, bool) {
	if want != "" {
		for _, v := range vs {
			if v.ID == want || v.Number == want {
				return v, true
			}
		}
		return modplatform.Version{}, false
	}
	for _, v := range vs {
		if v.Type == "release" {
			return v, true
		}
	}
	if len(vs) > 0 {
		return vs[0], true
	}
	return modplatform.Version{}, false
}

// Discard deletes the archive a pack was fetched into, once nothing needs
// it any more (packs read from the player's own files are left alone).
func (r *Reader) Discard(p *Pack) {
	if p != nil && p.download != nil {
		r.discardPending(p.download)
		p.download = nil
	}
	if p == nil || p.Archive == "" || r.d.Cache == "" {
		return
	}
	if rel, err := filepath.Rel(r.d.Cache, p.Archive); err == nil && filepath.IsLocal(rel) {
		_ = os.Remove(p.Archive)
	}
}

func (r *Reader) fetch(ctx context.Context, raw string, size int64, sum hashx.Sum) (string, error) {
	if r.d.Fetch == nil || r.d.Cache == "" {
		return "", ErrLink
	}
	if size > maxPackFile {
		return "", errors.New("modpack: the pack file is too large")
	}
	if err := os.MkdirAll(r.d.Cache, 0o700); err != nil {
		return "", err
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	dest := filepath.Join(r.d.Cache, hex.EncodeToString(b[:])+".zip")
	if err := r.d.Fetch(ctx, raw, size, sum, dest); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	if fi, err := os.Stat(dest); err != nil || fi.Size() > maxPackFile {
		_ = os.Remove(dest)
		return "", errors.New("modpack: the pack file is too large")
	}
	return dest, nil
}

// --- FTB ---

const ftbAPI = "https://api.feed-the-beast.com/v1/modpacks/public/modpack/"

// ftb reads an FTB pack through its API: the version asked for, or the newest release.
func (r *Reader) ftb(ctx context.Context, id, version string) (*Pack, error) {
	var pack struct {
		Status   string `json:"status"`
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Synopsis string `json:"synopsis"`
		Art      []struct {
			URL  string `json:"url"`
			Type string `json:"type"`
		} `json:"art"`
		Versions []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"versions"`
	}
	if err := r.d.HTTP.JSON(ctx, http.MethodGet, ftbAPI+id, nil, nil, &pack); err != nil || pack.Status == "error" || len(pack.Versions) == 0 {
		return nil, ErrLink
	}
	vid := version
	if vid == "" {
		best := pack.Versions[0]
		for _, v := range pack.Versions {
			if (strings.EqualFold(v.Type, "release") && (!strings.EqualFold(best.Type, "release") || v.ID > best.ID)) ||
				(!strings.EqualFold(best.Type, "release") && v.ID > best.ID) {
				best = v
			}
		}
		vid = strconv.FormatInt(best.ID, 10)
	}
	if _, err := strconv.ParseInt(vid, 10, 64); err != nil {
		return nil, ErrLink
	}
	var ver struct {
		Status  string `json:"status"`
		Name    string `json:"name"`
		Targets []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Type    string `json:"type"`
		} `json:"targets"`
		Files []struct {
			Path       string `json:"path"`
			Name       string `json:"name"`
			URL        string `json:"url"`
			SHA1       string `json:"sha1"`
			Size       int64  `json:"size"`
			ServerOnly bool   `json:"serveronly"`
			Optional   bool   `json:"optional"`
			Curse      *struct {
				Project flexInt `json:"project"`
				File    flexInt `json:"file"`
			} `json:"curseforge"`
		} `json:"files"`
	}
	if err := r.d.HTTP.JSON(ctx, http.MethodGet, ftbAPI+id+"/"+vid, nil, nil, &ver); err != nil || ver.Status == "error" {
		return nil, ErrLink
	}
	p := &Pack{Origin: Origin{Source: "ftb", Project: id, Version: vid}, Format: "FTB", Name: clip(pack.Name, 80), Version: clip(ver.Name, 40), Summary: clip(pack.Synopsis, 300)}
	for _, a := range pack.Art {
		if a.Type == "square" && allowedURL(a.URL, nil) {
			p.IconURL = a.URL
		}
	}
	for _, t := range ver.Targets {
		switch t.Type {
		case "game":
			p.Minecraft = t.Version
		case "modloader":
			if _, ok := mmcLoaderNames[t.Name]; ok {
				p.Loader, p.LoaderVersion = t.Name, t.Version
			}
		}
	}
	if p.Minecraft == "" {
		return nil, ErrNoMinecraft
	}
	if len(ver.Files) > maxFiles {
		return nil, fmt.Errorf("modpack: the pack lists too many files")
	}
	var curse []cfRef
	for _, f := range ver.Files {
		if f.ServerOnly {
			continue
		}
		rel, err := CleanPath(strings.TrimSuffix(f.Path, "/") + "/" + f.Name)
		if err != nil {
			return nil, err
		}
		if f.URL == "" && f.Curse != nil {
			curse = append(curse, cfRef{project: int64(f.Curse.Project), file: int64(f.Curse.File), optional: f.Optional})
			continue
		}
		sha1 := strings.ToLower(f.SHA1)
		if !hexOf(sha1, 40) || !allowedURL(f.URL, nil) || f.Size < 0 || f.Size > maxFileSize {
			return nil, fmt.Errorf("modpack: %s has no usable download", f.Name)
		}
		if f.Optional {
			rel += ".disabled"
		}
		p.Files = append(p.Files, File{Path: rel, SHA1: sha1, Size: f.Size, URLs: []string{f.URL}, Optional: f.Optional})
	}
	if err := r.resolveCurse(ctx, p, curse); err != nil {
		return nil, err
	}
	return p, nil
}

// flexInt reads a number FTB sends as either a JSON number or a string.
type flexInt int64

func (n *flexInt) UnmarshalJSON(b []byte) error {
	v, err := strconv.ParseInt(strings.Trim(string(b), `"`), 10, 64)
	if err != nil {
		return err
	}
	*n = flexInt(v)
	return nil
}

// mmcLoaderNames are the loader names FTB targets use.
var mmcLoaderNames = map[string]bool{"forge": true, "neoforge": true, "fabric": true, "quilt": true}

// --- Technic ---

const technicAPI = "https://api.technicpack.net/modpack/"

// technic reads a Technic pack through its API: a zip pack is fetched and
// read; a Solder pack lists mod zips that are unpacked into the game folder.
func (r *Reader) technic(ctx context.Context, slug string) (*Pack, error) {
	var info struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		URL         string `json:"url"`
		Solder      string `json:"solder"`
		Minecraft   string `json:"minecraft"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Icon        struct {
			URL string `json:"url"`
		} `json:"icon"`
		Error string `json:"error"`
	}
	if err := r.d.HTTP.JSON(ctx, http.MethodGet, technicAPI+slug+"?build=quartz", nil, nil, &info); err != nil || info.Error != "" {
		return nil, ErrLink
	}
	hint := &Pack{Origin: Origin{Source: "technic", Project: slug}, Format: "Technic", Name: clip(orElse(info.DisplayName, info.Name), 80),
		Version: clip(info.Version, 40), Minecraft: info.Minecraft, IconURL: info.Icon.URL}
	if s := strings.TrimRight(info.Solder, "/"); s != "" {
		return r.solder(ctx, s, info.Name, hint)
	}
	if !allowedURL(info.URL, nil) {
		return nil, ErrLink
	}
	dest, err := r.fetch(ctx, info.URL, 0, hashx.Sum{})
	if err != nil {
		return nil, err
	}
	p, err := r.File(ctx, dest)
	if err != nil {
		return nil, err
	}
	p.Origin, p.Name, p.Version, p.IconURL = hint.Origin, orElse(hint.Name, p.Name), orElse(hint.Version, p.Version), hint.IconURL
	if p.Minecraft == "" {
		p.Minecraft = hint.Minecraft
	}
	return p, nil
}

// solder reads a Technic Solder build: mod zips with md5 checksums, from
// the pack author's own server, unpacked into the game folder.
func (r *Reader) solder(ctx context.Context, base, name string, hint *Pack) (*Pack, error) {
	if !allowedURL(base, nil) || !technicPath.MatchString("/modpack/"+name) {
		return nil, ErrLink
	}
	var builds struct {
		Recommended string `json:"recommended"`
		Latest      string `json:"latest"`
	}
	if err := r.d.HTTP.JSON(ctx, http.MethodGet, base+"/modpack/"+name, nil, nil, &builds); err != nil {
		return nil, ErrLink
	}
	build := orElse(builds.Recommended, builds.Latest)
	if build == "" || strings.ContainsAny(build, "/?#\\") {
		return nil, ErrLink
	}
	var b struct {
		Minecraft string `json:"minecraft"`
		Forge     string `json:"forge"`
		Mods      []struct {
			Name     string `json:"name"`
			Version  string `json:"version"`
			MD5      string `json:"md5"`
			URL      string `json:"url"`
			Filesize int64  `json:"filesize"`
		} `json:"mods"`
	}
	if err := r.d.HTTP.JSON(ctx, http.MethodGet, base+"/modpack/"+name+"/"+url.PathEscape(build), nil, nil, &b); err != nil {
		return nil, ErrLink
	}
	p := hint
	p.Version = orElse(build, p.Version)
	p.Minecraft = orElse(b.Minecraft, p.Minecraft)
	if b.Forge != "" {
		p.Loader, p.LoaderVersion = "forge", b.Forge
	}
	if p.Minecraft == "" {
		return nil, ErrNoMinecraft
	}
	if len(b.Mods) > maxFiles {
		return nil, fmt.Errorf("modpack: the pack lists too many files")
	}
	for i, m := range b.Mods {
		md5 := strings.ToLower(m.MD5)
		if !hexOf(md5, 32) || !allowedURL(m.URL, nil) || m.Filesize < 0 || m.Filesize > maxFileSize {
			return nil, fmt.Errorf("modpack: %s has no usable download", clip(m.Name, 40))
		}
		p.Files = append(p.Files, File{Path: fmt.Sprintf(".solder/%d.zip", i), MD5: md5, Size: m.Filesize, URLs: []string{m.URL},
			Name: clip(m.Name, 80), Extract: true})
	}
	p.Warnings = append(p.Warnings, "This pack's files come from its author's own server, checked only by md5.")
	return p, nil
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
