package app

import "testing"

// The frame is the resize borders plus the title bar: wider and taller than
// nothing, and far from a page's size.
func TestWindowFrame(t *testing.T) {
	w, h := windowFrame()
	if w <= 0 || w > 64 || h <= w || h > 96 {
		t.Errorf("windowFrame() = %d×%d, want a few pixels wide and a title bar taller", w, h)
	}
}
