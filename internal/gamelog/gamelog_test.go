package gamelog

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeRedactor struct{}

func (fakeRedactor) Redact(s string) string { return strings.ReplaceAll(s, "SECRET", "[redacted]") }

func collect(stderr bool, input string) []Line {
	var out []Line
	p := NewParser(func(l Line) { out = append(out, l) }, fakeRedactor{}, stderr)
	p.now = func() time.Time { return time.Date(2026, 1, 1, 9, 8, 7, 0, time.Local) }
	for _, l := range strings.Split(input, "\n") {
		p.Feed(l)
	}
	p.Close()
	return out
}

func TestXMLEvents(t *testing.T) {
	ts := time.Date(2026, 1, 1, 12, 34, 56, 0, time.Local).UnixMilli()
	in := `<log4j:Event logger="net.minecraft.client.Minecraft" timestamp="` + itoa(ts) + `" level="INFO" thread="Render thread">
  <log4j:Message><![CDATA[Setting user: Steve]]></log4j:Message>
</log4j:Event>
plain line from println
<log4j:Event logger="x" timestamp="` + itoa(ts) + `" level="ERROR" thread="Worker-1">
  <log4j:Message><![CDATA[Crash with SECRET & <stuff>]]></log4j:Message>
  <log4j:Throwable><![CDATA[java.lang.RuntimeException: boom
	at a.b.C.d(C.java:1)
]]></log4j:Throwable>
</log4j:Event>`
	got := collect(false, in)
	want := []Line{
		{Time: "12:34:56", Level: Info, Thread: "Render thread", Message: "Setting user: Steve"},
		{Time: "09:08:07", Level: Info, Message: "plain line from println"},
		{Time: "12:34:56", Level: Error, Thread: "Worker-1", Message: "Crash with [redacted] & <stuff>", Redacted: true},
		{Time: "12:34:56", Level: Error, Thread: "Worker-1", Message: "java.lang.RuntimeException: boom"},
		{Time: "12:34:56", Level: Error, Thread: "Worker-1", Message: "\tat a.b.C.d(C.java:1)"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLegacyAndStderr(t *testing.T) {
	got := collect(true, "[10:00:01] [main/WARN]: Old warning\n2013-09-19 12:00:00 [INFO] Very old\nraw stderr")
	if got[0].Level != Warn || got[0].Thread != "main" || got[0].Time != "10:00:01" || got[0].Message != "Old warning" {
		t.Errorf("bracketed: %+v", got[0])
	}
	if got[1].Level != Info || got[1].Time != "12:00:00" || got[1].Message != "Very old" {
		t.Errorf("old style: %+v", got[1])
	}
	if got[2].Level != Error {
		t.Errorf("stderr default: %+v", got[2])
	}
}

func TestMalformedFallsBackToPlain(t *testing.T) {
	got := collect(false, "<log4j:Event level=\"INFO\" thread=\"x\">\n  <log4j:Message><![CDATA[unterminated\n</log4j:Event>")
	if len(got) == 0 {
		t.Fatal("malformed event swallowed")
	}
	got = collect(false, "<log4j:Event level=\"INFO\">\nnever closed")
	if len(got) != 2 {
		t.Fatalf("unclosed event not flushed at Close: %+v", got)
	}
}

func TestBuffer(t *testing.T) {
	b := NewBuffer(3)
	for i := range 5 {
		b.Add(Line{Message: itoa(int64(i))})
	}
	got := b.Lines()
	if len(got) != 3 || got[0].Message != "2" || got[2].Message != "4" {
		t.Fatalf("ring = %+v", got)
	}
}

func FuzzParser(f *testing.F) {
	f.Add("<log4j:Event level=\"INFO\"><log4j:Message><![CDATA[x]]></log4j:Message></log4j:Event>")
	f.Add("[10:00:01] [main/WARN]: x")
	f.Add("<log4j:Event\n<<<]]>&&;")
	f.Fuzz(func(t *testing.T, in string) {
		_ = collect(false, in)
	})
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestForgeConsoleLayout(t *testing.T) {
	got := collect(false, "[13Oct2026 17:11:47.924] [Server thread/INFO] [net.minecraft.server.dedicated.DedicatedServer/]: Done (3.381s)! For help, type \"help\"")
	if len(got) != 1 || got[0].Time != "17:11:47" || got[0].Thread != "Server thread" || got[0].Level != Info || got[0].Message != `Done (3.381s)! For help, type "help"` {
		t.Fatalf("%+v", got)
	}
}
