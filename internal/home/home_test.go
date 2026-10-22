package home

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"quartz/internal/brand"
	"quartz/internal/core/httpcache"
	"quartz/internal/core/remoteimg"
	"quartz/internal/launch"
	"quartz/internal/meta"
)

func TestCountWeek(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	now := time.Date(2026, 10, 14, 15, 0, 0, 0, loc) // a Wednesday
	at := func(d, h, m int) int64 { return time.Date(2026, 10, d, h, m, 0, 0, loc).UnixMilli() }
	recs := []launch.Record{
		{StartedAt: at(12, 20, 0), LengthMin: 85},  // Monday
		{StartedAt: at(13, 23, 30), LengthMin: 60}, // Tuesday 30 min, Wednesday 30 min
		{StartedAt: at(11, 22, 0), LengthMin: 600}, // last Sunday: only the part after Monday 00:00
		{StartedAt: at(5, 12, 0), LengthMin: 60},   // last week
	}
	days := Count(recs, now)
	want := []int{85 + 480, 30, 30, 0, 0, 0, 0}
	for i, d := range days {
		if d.Minutes != want[i] || d.Day != i || d.Today != (i == 2) {
			t.Errorf("day %d: %+v, want %d minutes", i, d, want[i])
		}
	}
	// On a Sunday, the highlight is the last bar.
	sun := Count(nil, time.Date(2026, 10, 18, 9, 0, 0, 0, loc))
	if !sun[6].Today {
		t.Fatal("Sunday isn't the last bar")
	}
}

func TestPickNews(t *testing.T) {
	var f feed
	_ = json.Unmarshal([]byte(`{"entries":[
		{"id":"a","title":"Minecraft Realms","category":"Minecraft for Windows","date":"2026-10-08","readMoreLink":"https://www.minecraft.net/article/x","newsType":["News page","Bedrock"]},
		{"id":"b","title":"New on Java Realms","category":"Minecraft: Java Edition","date":"2026-09-28","readMoreLink":"https://www.minecraft.net/article/y?OCID=Launcher","newsType":["News page","Java"],"newsPageImage":{"url":"/v2/images/jr.jpg"}},
		{"id":"c","title":"Java 26.2 is out","category":"Minecraft: Java Edition","date":"2026-09-16","readMoreLink":"https://www.minecraft.net/article/z","newsType":["Java"]},
		{"id":"d","title":"Evil","category":"Minecraft: Java Edition","date":"2026-10-12","readMoreLink":"javascript:alert(1)","newsType":["Java"]},
		{"id":"e","title":"Phish","category":"Minecraft: Java Edition","date":"2026-10-12","readMoreLink":"https://minecraft.net.evil.com/x","newsType":["Java"]}
	]}`), &f)
	n := pick(f, nil)
	if n == nil || n.ID != "b" || n.Source != "minecraft.net" {
		t.Fatalf("%+v", n)
	}
}

func TestPickNote(t *testing.T) {
	var n patchNotes
	_ = json.Unmarshal([]byte(`{"version":1,"entries":[
		{"title":"Minecraft 26.4 Snapshot 3","id":"8df0","date":"2026-10-06T12:54:26.000Z","image":{"url":"/v2/images/s3.jpg"}},
		{"title":"Minecraft 26.4 Snapshot 4","id":"26-4-snapshot-4","date":"2026-10-13T12:05:46.000Z","image":{"url":"/v2/images/s4.jpg"}},
		{"title":"???","id":"x","date":"2026-10-14T00:00:00.000Z"},
		{"title":"Minecraft 27","id":"y","date":"not a date"}
	]}`), &n)
	p := pickNote(n, nil)
	if p == nil || p.ID != "26-4-snapshot-4" || p.URL != "https://www.minecraft.net/en-us/article/minecraft-26-4-snapshot-4" {
		t.Fatalf("%+v", p)
	}
	// The newer of an article and a patch note wins.
	a := &News{ID: "a", Published: p.Published - 1}
	if newer(a, p) != p || newer(p, a) != p || newer(nil, a) != a || newer(a, nil) != a {
		t.Fatal("newer picked the older one")
	}
}

func TestArticleSlug(t *testing.T) {
	for title, want := range map[string]string{
		"Minecraft 26.4 Snapshot 4":          "minecraft-26-4-snapshot-4",
		"Minecraft: Java Edition 26.3":       "minecraft-java-edition-26-3",
		"Minecraft 26.3 Release Candidate 3": "minecraft-26-3-release-candidate-3",
		"26.3-snapshot-10":                   "minecraft-26-3-snapshot-10",
		"  Minecraft 26.3 Pre-Release 1 ":    "minecraft-26-3-pre-release-1",
		"../../evil?x=<b>":                   "minecraft-evil-x-b",
		"Minecraft":                          "",
		"???":                                "",
	} {
		if got := articleSlug(title); got != want {
			t.Errorf("%q: got %q, want %q", title, got, want)
		}
	}
}

func TestPickPosts(t *testing.T) {
	var p posts
	_ = json.Unmarshal([]byte(`{"entries":[
		{"id":"old","title":"Beta","published":"2026-09-01T00:00:00Z","url":"https://quartz.conspiracy.moe/beta","image":"https://quartz.conspiracy.moe/news/beta.png"},
		{"id":"new","title":" Quartz 1.0 ","published":"2026-10-22T12:00:00Z","url":"https://example.com/launch","image":"https://evil.example/x.png"},
		{"id":"js","title":"Evil","published":"2026-10-22T00:00:00Z","url":"javascript:alert(1)"},
		{"id":"http","title":"Plain","published":"2026-10-22T00:00:00Z","url":"http://quartz.conspiracy.moe/"},
		{"id":"user","title":"Phish","published":"2026-10-22T00:00:00Z","url":"https://quartz.conspiracy.moe@evil.example/"},
		{"id":"date","title":"Undated","published":"yesterday","url":"https://quartz.conspiracy.moe/"},
		{"id":"","title":"No id","published":"2026-10-22T00:00:00Z","url":"https://quartz.conspiracy.moe/"},
		{"id":"empty","title":"  ","published":"2026-10-22T00:00:00Z","url":"https://quartz.conspiracy.moe/"},
		{"id":"long","title":"`+strings.Repeat("x", maxTitle+1)+`","published":"2026-10-22T00:00:00Z","url":"https://quartz.conspiracy.moe/"}
	]}`), &p)
	pics := remoteimg.New(t.TempDir(), nil, nil)
	got := pickPosts(p, pics)
	// The feed's order stays, so the server can pin a post by listing it first.
	if len(got) != 2 || got[0].ID != "quartz:old" || got[1].ID != "quartz:new" || got[1].Title != "Quartz 1.0" {
		t.Fatalf("%+v", got)
	}
	if !strings.HasPrefix(got[0].Image, "/tex/remote/") || got[1].Image != "" || got[1].Source != brand.ShortName {
		t.Fatalf("%+v", got)
	}
	// A flood is cut to the first few.
	p.Entries = slices.Repeat(p.Entries[:1], maxPosts+3)
	if n := len(pickPosts(p, nil)); n != maxPosts {
		t.Fatal(n)
	}
}

// feeds answers each URL with a fixed status and body.
type feeds map[string]string

func (f feeds) Do(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	if body, ok := f[r.URL.String()]; ok {
		_, _ = w.WriteString(body)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
	return w.Result(), nil
}

func newsService(t *testing.T, f feeds) *Service {
	t.Helper()
	cache, err := httpcache.Open(t.TempDir(), f, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Meta: meta.New(cache)})
}

func TestNewsPutsMinecraftFirst(t *testing.T) {
	s := newsService(t, feeds{
		PatchNotes: `{"entries":[{"title":"Minecraft 26.4 Snapshot 4","id":"s4","date":"2026-10-13T12:05:46.000Z"}]}`,
		QuartzNews: `{"entries":[{"id":"launch","title":"Quartz 1.0","published":"2026-10-22T12:00:00Z","url":"https://quartz.conspiracy.moe/"}]}`,
	})
	got, err := s.News(t.Context())
	if err != nil || len(got) != 2 || got[0].ID != "s4" || got[1].ID != "quartz:launch" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestNewsWithoutOwnFeed(t *testing.T) {
	// The launcher's feed isn't there (404): Minecraft news still shows.
	s := newsService(t, feeds{PatchNotes: `{"entries":[{"title":"Minecraft 26.4 Snapshot 4","id":"s4","date":"2026-10-13T12:05:46.000Z"}]}`})
	got, err := s.News(t.Context())
	if err != nil || len(got) != 1 || got[0].ID != "s4" {
		t.Fatalf("%+v %v", got, err)
	}
	// Nothing reachable at all is an error.
	if _, err := newsService(t, feeds{}).News(t.Context()); err == nil {
		t.Fatal("no error without any feed")
	}
}
