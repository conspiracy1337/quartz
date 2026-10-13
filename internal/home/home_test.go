package home

import (
	"encoding/json"
	"testing"
	"time"

	"quartz/internal/launch"
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
