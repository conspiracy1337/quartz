package events

import "testing"

func TestBus(t *testing.T) {
	b := NewBus()
	var got []any
	var sunk []Topic
	unsub := b.Subscribe(SettingsChanged, func(p any) { got = append(got, p) })
	b.SetSink(func(t Topic, _ any) { sunk = append(sunk, t) })
	b.Emit(SettingsChanged, 1)
	b.Emit(CustomCSS, 2)
	unsub()
	b.Emit(SettingsChanged, 3)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("subscriber got %v", got)
	}
	if len(sunk) != 3 {
		t.Fatalf("sink got %v", sunk)
	}
}
