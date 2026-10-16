package mojang

import (
	"regexp"
	"sort"
	"strconv"
	"time"
)

// ManifestURL is Mojang's list of every version.
const ManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

// Manifest is the version manifest (v2, with sha1 per version JSON).
type Manifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []ManifestEntry `json:"versions"`
}

// ManifestEntry points to one version JSON.
type ManifestEntry struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // release | snapshot | old_beta | old_alpha
	URL         string    `json:"url"`
	SHA1        string    `json:"sha1"`
	ReleaseTime time.Time `json:"releaseTime"`
}

// Find returns the entry for id.
func (m Manifest) Find(id string) (ManifestEntry, bool) {
	for _, v := range m.Versions {
		if v.ID == id {
			return v, true
		}
	}
	return ManifestEntry{}, false
}

// OldestSupported is 1.6.1's release: versions 1.5 and older need the applet
// launcher and were cut from scope.
var OldestSupported = time.Date(2013, 6, 28, 14, 48, 41, 0, time.UTC)

// Supported reports whether the launcher can run this version.
func (e ManifestEntry) Supported() bool {
	return (e.Type == "release" || e.Type == "snapshot") && !e.ReleaseTime.Before(OldestSupported)
}

// javaThresholds are the first release times needing each Java major,
// measured from Mojang's own version JSONs (26.1-snapshot-1, 24w14a,
// 1.18-pre2, 21w19a). A version JSON, when cached, always wins.
var javaThresholds = []struct {
	from  time.Time
	major int
}{
	{time.Date(2025, 12, 16, 12, 42, 29, 0, time.UTC), 25},
	{time.Date(2024, 4, 3, 11, 49, 39, 0, time.UTC), 21},
	{time.Date(2021, 11, 16, 17, 4, 48, 0, time.UTC), 17},
	{time.Date(2021, 5, 12, 11, 19, 15, 0, time.UTC), 16},
}

// EstimatedJava returns the Java major a version most likely needs.
func (e ManifestEntry) EstimatedJava() int {
	for _, t := range javaThresholds {
		if !e.ReleaseTime.Before(t.from) {
			return t.major
		}
	}
	return 8
}

// Release is one entry in the version picker.
type Release struct {
	ID       string `json:"id"`
	Java     int    `json:"java"`
	Released int64  `json:"released"` // unix ms
	Snapshot bool   `json:"snapshot,omitempty"`
}

// Family groups releases for the two-step version picker: "1.21", "26.x".
type Family struct {
	ID        string    `json:"id"`
	Note      string    `json:"note"`
	Releases  []Release `json:"releases"`
	Snapshots []Release `json:"snapshots"`
}

var (
	releaseID  = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.\d+)?$`)
	prefixedID = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.\d+)?-`)
)

// familyOf names the family of a release id: 1.21.4 → "1.21", 26.1.2 → "26.x".
func familyOf(id string) (string, bool) {
	m := releaseID.FindStringSubmatch(id)
	if m == nil {
		m = prefixedID.FindStringSubmatch(id)
	}
	if m == nil {
		return "", false
	}
	if m[1] == "1" {
		return "1." + m[2], true
	}
	return m[1] + ".x", true
}

// Families groups supported versions newest first. Pre-releases and snapshots
// named after a release ("26.4-snapshot-3", "1.21.5-pre1") join that family;
// weekly snapshots ("25w14a") join the next release after them. java gives
// the Java major for an entry (cached JSON truth or the estimate).
func Families(m Manifest, java func(ManifestEntry) int) []Family {
	var order []string
	byID := map[string]*Family{}
	years := map[string][2]int{}
	get := func(id string) *Family {
		if f, ok := byID[id]; ok {
			return f
		}
		byID[id] = &Family{ID: id, Releases: []Release{}, Snapshots: []Release{}}
		order = append(order, id)
		return byID[id]
	}
	entries := append([]ManifestEntry(nil), m.Versions...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ReleaseTime.After(entries[j].ReleaseTime) })
	next := "" // family of the closest newer release, for weekly snapshots
	for _, e := range entries {
		if !e.Supported() {
			continue
		}
		r := Release{ID: e.ID, Java: java(e), Released: e.ReleaseTime.UnixMilli(), Snapshot: e.Type == "snapshot"}
		fam, ok := familyOf(e.ID)
		if e.Type == "release" && ok {
			next = fam
			f := get(fam)
			f.Releases = append(f.Releases, r)
			y := e.ReleaseTime.Year()
			span := years[fam]
			if span[0] == 0 || y < span[0] {
				span[0] = y
			}
			span[1] = max(span[1], y)
			years[fam] = span
			continue
		}
		if !ok {
			fam = next
		}
		if fam == "" {
			continue
		}
		f := get(fam)
		f.Snapshots = append(f.Snapshots, r)
	}
	out := make([]Family, 0, len(order))
	for i, id := range order {
		f := byID[id]
		if len(f.Releases) == 0 {
			continue // snapshots of a release that does not exist yet stay hidden
		}
		switch span := years[id]; {
		case i == 0:
			f.Note = "Newest"
		case span[0] == span[1]:
			f.Note = strconv.Itoa(span[0])
		default:
			f.Note = strconv.Itoa(span[0]) + " to " + strconv.Itoa(span[1])
		}
		out = append(out, *f)
	}
	return out
}
