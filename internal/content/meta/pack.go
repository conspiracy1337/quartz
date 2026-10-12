package meta

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sync"

	"quartz/internal/core/archive"
)

// Format is a pack format: a major number and, from 1.21.9, a minor one.
type Format [2]int

// Less orders formats.
func (f Format) Less(o Format) bool { return f[0] < o[0] || (f[0] == o[0] && f[1] < o[1]) }

// PackFormat is the range of formats a pack says it supports.
type PackFormat struct {
	Min Format `json:"min"`
	Max Format `json:"max"`
}

// Supports reports whether a game with format f accepts the pack.
func (p PackFormat) Supports(f Format) bool { return !f.Less(p.Min) && !p.Max.Less(f) }

// Pack reads a resource or data pack (a zip or a folder): pack.mcmeta for
// the description and formats, pack.png for the icon.
func Pack(src Source, ic Icons) (Info, error) {
	data, err := src.Read("pack.mcmeta", maxText)
	if err != nil {
		return Info{}, ErrNotContent
	}
	var mc struct {
		Pack struct {
			Description json.RawMessage `json:"description"`
			PackFormat  json.RawMessage `json:"pack_format"`
			Supported   json.RawMessage `json:"supported_formats"`
			MinFormat   json.RawMessage `json:"min_format"`
			MaxFormat   json.RawMessage `json:"max_format"`
		} `json:"pack"`
	}
	if err := decodeJSON(data, &mc); err != nil {
		return Info{}, ErrNotContent
	}
	p := mc.Pack
	info := Info{Format: FormatPack, Description: clean(component(p.Description), maxDesc), Icon: icon(src, "pack.png", ic)}
	if pf := packFormats(p.PackFormat, p.Supported, p.MinFormat, p.MaxFormat); pf != nil {
		info.Pack = pf
	}
	return info, nil
}

// packFormats reads the formats every pack.mcmeta generation used:
// pack_format, supported_formats (1.20.2: a number, [min, max] or
// {min_inclusive, max_inclusive}) and min_format/max_format (1.21.9: a
// number or [major, minor]).
func packFormats(packFormat, supported, minF, maxF json.RawMessage) *PackFormat {
	var out PackFormat
	set := false
	widen := func(lo, hi Format) {
		if !set {
			out, set = PackFormat{Min: lo, Max: hi}, true
			return
		}
		if lo.Less(out.Min) {
			out.Min = lo
		}
		if out.Max.Less(hi) {
			out.Max = hi
		}
	}
	if f, ok := formatValue(packFormat); ok {
		widen(f, upTo(f))
	}
	if len(supported) > 0 {
		var n int
		var pair []int
		var obj struct {
			Min int `json:"min_inclusive"`
			Max int `json:"max_inclusive"`
		}
		switch {
		case json.Unmarshal(supported, &n) == nil:
			widen(Format{n}, upTo(Format{n}))
		case json.Unmarshal(supported, &pair) == nil && len(pair) == 2:
			widen(Format{pair[0]}, upTo(Format{pair[1]}))
		case json.Unmarshal(supported, &obj) == nil && obj.Max > 0:
			widen(Format{obj.Min}, upTo(Format{obj.Max}))
		}
	}
	lo, okLo := formatValue(minF)
	hi, okHi := formatValue(maxF)
	switch {
	case okLo && okHi:
		widen(lo, upTo(hi))
	case okLo:
		widen(lo, upTo(lo))
	case okHi:
		widen(hi, upTo(hi))
	}
	if !set || out.Max[0] <= 0 {
		return nil
	}
	return &out
}

// anyMinor marks a maximum given without a minor number: every minor of
// that major fits.
const anyMinor = 1 << 20

// upTo turns a format into the highest one it allows as a maximum.
func upTo(f Format) Format {
	if f[1] == 0 {
		f[1] = anyMinor
	}
	return f
}

// formatValue reads 34 or [97, 1].
func formatValue(raw json.RawMessage) (Format, bool) {
	if len(raw) == 0 {
		return Format{}, false
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		return Format{int(n)}, true
	}
	var pair []int
	if json.Unmarshal(raw, &pair) == nil && len(pair) >= 1 && pair[0] > 0 {
		f := Format{pair[0]}
		if len(pair) > 1 {
			f[1] = pair[1]
		}
		return f, true
	}
	return Format{}, false
}

// Shader reads a shader pack: it is one when it has a shaders/ folder. Shader
// packs carry no metadata, so the name comes from the file.
func Shader(src Source) (Info, error) {
	if !src.HasPrefix("shaders/") {
		return Info{}, ErrNotContent
	}
	return Info{Format: FormatShader}, nil
}

// ClientFormats are the pack formats a Minecraft version reads.
type ClientFormats struct {
	Resource Format `json:"resource"`
	Data     Format `json:"data"`
	// HasData is false when the version only published one number (before
	// 1.19.3), which can't be trusted for data packs.
	HasData bool `json:"hasData"`
}

// ClientPack reads the formats from a client jar's version.json. Versions
// before 1.14 have none and report ok=false.
func ClientPack(clientJar string) (ClientFormats, bool) {
	r, err := archive.Open(clientJar)
	if err != nil {
		return ClientFormats{}, false
	}
	defer func() { _ = r.Close() }()
	data, err := r.Read("version.json", maxText)
	if err != nil {
		return ClientFormats{}, false
	}
	var v struct {
		PackVersion json.RawMessage `json:"pack_version"`
	}
	if json.Unmarshal(data, &v) != nil {
		return ClientFormats{}, false
	}
	var n int
	if json.Unmarshal(v.PackVersion, &n) == nil && n > 0 {
		return ClientFormats{Resource: Format{n}, Data: Format{n}}, true
	}
	var pv struct {
		Resource      int `json:"resource"`
		Data          int `json:"data"`
		ResourceMajor int `json:"resource_major"`
		ResourceMinor int `json:"resource_minor"`
		DataMajor     int `json:"data_major"`
		DataMinor     int `json:"data_minor"`
	}
	if json.Unmarshal(v.PackVersion, &pv) != nil {
		return ClientFormats{}, false
	}
	if pv.ResourceMajor > 0 {
		return ClientFormats{Resource: Format{pv.ResourceMajor, pv.ResourceMinor}, Data: Format{pv.DataMajor, pv.DataMinor}, HasData: pv.DataMajor > 0}, true
	}
	if pv.Resource > 0 {
		return ClientFormats{Resource: Format{pv.Resource}, Data: Format{pv.Data}, HasData: pv.Data > 0}, true
	}
	return ClientFormats{}, false
}

//go:embed formats.json
var formatsJSON []byte

// Which game versions each format was used by, from Prism's tables
// (minecraft/mod/ResourcePack.cpp and DataPack.cpp).
var formatTable = sync.OnceValue(func() map[string]map[Format]string {
	var raw map[string][][4]any
	out := map[string]map[Format]string{}
	if json.Unmarshal(formatsJSON, &raw) != nil {
		return out
	}
	for kind, rows := range raw {
		m := map[Format]string{}
		for _, r := range rows {
			maj, _ := r[0].(float64)
			minor, _ := r[1].(float64)
			first, _ := r[2].(string)
			m[Format{int(maj), int(minor)}] = first
		}
		out[kind] = m
	}
	return out
})

var family = regexp.MustCompile(`^\d+\.\d+`)

// Label says which version a pack that doesn't fit game was made for, such
// as "Made for 1.20", or "" when it fits. kind is "resource" or "data".
func Label(kind string, p *PackFormat, game Format) string {
	if p == nil || game[0] == 0 || p.Supports(game) {
		return ""
	}
	at, word := p.Max, "an older"
	if game.Less(p.Min) {
		at, word = p.Min, "a newer"
	}
	if at[1] == anyMinor {
		at[1] = 0
	}
	if first, ok := formatTable()[kind][at]; ok {
		if f := family.FindString(first); f != "" {
			return "Made for " + f
		}
	}
	return "Made for " + word + " version"
}
