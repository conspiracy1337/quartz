package modplatform

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// fakeProvider answers Latest from a table: loader → key → version.
type fakeProvider struct {
	answers map[string]map[string]Version
	asked   []string
	fail    string
}

func (f *fakeProvider) Source() Source { return Modrinth }
func (f *fakeProvider) Ready() bool    { return true }
func (f *fakeProvider) Identify(context.Context, []Hashes) (map[string]Version, error) {
	return nil, nil
}
func (f *fakeProvider) Projects(context.Context, []string) (map[string]Project, error) {
	return nil, nil
}
func (f *fakeProvider) Versions(context.Context, string, Query) ([]Version, error) { return nil, nil }
func (f *fakeProvider) Changelogs(context.Context, []Version) error                { return nil }
func (f *fakeProvider) Latest(_ context.Context, items []Installed, loader string, _ Query) (map[string]Version, error) {
	keys := ""
	for _, it := range items {
		keys += it.Key
	}
	f.asked = append(f.asked, loader+":"+keys)
	if f.fail != "" && loader == f.fail {
		return nil, errors.New("boom")
	}
	out := map[string]Version{}
	for _, it := range items {
		if v, ok := f.answers[loader][it.Key]; ok {
			out[it.Key] = v
		}
	}
	return out, nil
}

var t0 = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

func ver(id, typ string, days int) Version {
	return Version{ID: id, Type: typ, Published: t0.AddDate(0, 0, days), File: File{SHA1: id}}
}

func TestCheckLoaderChainAndForcedPass(t *testing.T) {
	p := &fakeProvider{answers: map[string]map[string]Version{
		"quilt":  {"a": ver("a2", "release", 2)},
		"fabric": {"b": ver("b2", "release", 2)},
		"forge":  {"c": ver("c2", "release", 2)},
	}}
	items := []Installed{
		{Key: "a", Current: ver("a1", "release", 1)},
		{Key: "b", Current: ver("b1", "release", 1)},
		{Key: "c", Current: ver("c1", "release", 1), Loaders: []string{"forge"}},
	}
	got, err := Check(context.Background(), p, items, Query{Loaders: []string{"quilt", "fabric"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["a"].ID != "a2" || got["b"].ID != "b2" || got["c"].ID != "c2" {
		t.Fatalf("%+v", got)
	}
	// Answered items drop out; the forced pass only asks items that declare it.
	want := []string{"quilt:abc", "fabric:bc", "forge:c"}
	if !reflect.DeepEqual(p.asked, want) {
		t.Fatalf("asked %v, want %v", p.asked, want)
	}
}

func TestCheckRulesOutNonUpdates(t *testing.T) {
	p := &fakeProvider{answers: map[string]map[string]Version{"": {
		"same":  ver("v1", "release", 1),
		"older": ver("old", "release", 0), // installed is a newer beta
		"beta":  ver("b2", "beta", 5),
		"newer": ver("n2", "release", 3),
	}}}
	items := []Installed{
		{Key: "same", Current: ver("v1", "release", 1)},
		{Key: "older", Current: ver("beta1", "beta", 2)},
		{Key: "beta", Current: ver("b1", "release", 1)},
		{Key: "newer", Current: ver("n1", "release", 1)},
	}
	got, _ := Check(context.Background(), p, items, Query{Channel: Release})
	if len(got) != 1 || got["newer"].ID != "n2" {
		t.Fatalf("release channel: %+v", got)
	}
	got, _ = Check(context.Background(), p, items, Query{Channel: Beta})
	if len(got) != 2 || got["beta"].ID != "b2" {
		t.Fatalf("beta channel: %+v", got)
	}
}

func TestCheckKeepsGoingAfterAFailure(t *testing.T) {
	p := &fakeProvider{fail: "fabric", answers: map[string]map[string]Version{"forge": {"a": ver("a2", "release", 2)}}}
	got, err := Check(context.Background(), p, []Installed{{Key: "a", Current: ver("a1", "release", 1)}}, Query{Loaders: []string{"fabric", "forge"}})
	if err == nil || got["a"].ID != "a2" {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestChannel(t *testing.T) {
	if ParseChannel("beta") != Beta || ParseChannel("nonsense") != Release || !Alpha.Allows("beta") || Release.Allows("beta") {
		t.Fatal("channels")
	}
}

func TestItems(t *testing.T) {
	md := "# Lithium 0.13\n\nIntro paragraph.\n\n## Additions\n- new: **LongJump** optimisation ([#12](https://x))\n* fixes `goats`\n1. third\n"
	if got := Items(md, false); !reflect.DeepEqual(got, []string{"new: LongJump optimisation (#12)", "fixes goats", "third"}) {
		t.Fatalf("%q", got)
	}
	if got := Items("Just one paragraph.\n\nAnd another.", false); !reflect.DeepEqual(got, []string{"Just one paragraph.", "And another."}) {
		t.Fatalf("%q", got)
	}
	h := `<p>Changes:</p><ul><li>Fixes <b>crash</b> on AMD</li><li>Faster &amp; smaller</li></ul><script>alert(1)</script>`
	if got := Items(h, true); !reflect.DeepEqual(got, []string{"Fixes crash on AMD", "Faster & smaller"}) {
		t.Fatalf("%q", got)
	}
	if got := Items("<p>Line one<br>Line two</p>", true); !reflect.DeepEqual(got, []string{"Line one", "Line two"}) {
		t.Fatalf("%q", got)
	}
}

func TestValidURLAndID(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://cdn.modrinth.com/data/x/y.jar":       true,
		"https://edge.forgecdn.net/files/1/2/a.jar":   true,
		"http://cdn.modrinth.com/data/x/y.jar":        false,
		"https://cdn.modrinth.com.evil.example/a.jar": false,
		"https://example.com/a.jar":                   false,
	} {
		if ValidURL(raw) != ok {
			t.Errorf("%s", raw)
		}
	}
	if !ValidID("AANobbMI") || !ValidID("327154") || ValidID("../x") || ValidID("") {
		t.Fatal("ids")
	}
}
