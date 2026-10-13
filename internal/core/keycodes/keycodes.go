// Package keycodes maps one key between the forms the launcher meets:
// options.txt before 1.13 (LWJGL2 key codes, mouse buttons from -100),
// options.txt since 1.13 (GLFW names like key.keyboard.left.control), the
// browser's KeyboardEvent.code (or "Mouse<n>" for MouseEvent.button), and a
// label for the UI.
package keycodes

import (
	"strconv"
	"strings"
)

// Key is one key in every form. Legacy is 0 when LWJGL2 had no code for it.
type Key struct {
	Name    string // options.txt since 1.13, e.g. key.keyboard.left.control
	Legacy  int    // options.txt before 1.13, e.g. 29
	Browser string // KeyboardEvent.code, or "Mouse<button>"
	Label   string // e.g. "Left Ctrl"
}

// Unknown is the name the game writes for an unbound key.
const Unknown = "key.keyboard.unknown"

var (
	byName    = map[string]Key{}
	byLegacy  = map[int]Key{}
	byBrowser = map[string]Key{}
)

func add(name string, legacy int, browser, label string) {
	k := Key{Name: name, Legacy: legacy, Browser: browser, Label: label}
	byName[name] = k
	if legacy != 0 {
		byLegacy[legacy] = k
	}
	if browser != "" {
		byBrowser[browser] = k
	}
}

func init() {
	add(Unknown, 0, "", "Not bound")
	// Letters: LWJGL2 numbers them by keyboard row.
	for i, c := range "qwertyuiop" {
		letter(c, 16+i)
	}
	for i, c := range "asdfghjkl" {
		letter(c, 30+i)
	}
	for i, c := range "zxcvbnm" {
		letter(c, 44+i)
	}
	for d := 0; d <= 9; d++ {
		legacy := 1 + d
		if d == 0 {
			legacy = 11
		}
		s := strconv.Itoa(d)
		add("key.keyboard."+s, legacy, "Digit"+s, s)
	}
	fLegacy := map[int]int{11: 87, 12: 88, 13: 100, 14: 101, 15: 102, 16: 103, 17: 104, 18: 105, 19: 113}
	for f := 1; f <= 25; f++ {
		s := strconv.Itoa(f)
		legacy := fLegacy[f]
		if f <= 10 {
			legacy = 58 + f
		}
		browser := "F" + s
		if f > 24 {
			browser = ""
		}
		add("key.keyboard.f"+s, legacy, browser, "F"+s)
	}
	pad := []int{82, 79, 80, 81, 75, 76, 77, 71, 72, 73}
	for d := 0; d <= 9; d++ {
		s := strconv.Itoa(d)
		add("key.keyboard.keypad."+s, pad[d], "Numpad"+s, "Keypad "+s)
	}
	for _, k := range []struct {
		name   string
		legacy int
		code   string
		label  string
	}{
		{"keypad.decimal", 83, "NumpadDecimal", "Keypad ."},
		{"keypad.divide", 181, "NumpadDivide", "Keypad /"},
		{"keypad.multiply", 55, "NumpadMultiply", "Keypad *"},
		{"keypad.subtract", 74, "NumpadSubtract", "Keypad -"},
		{"keypad.add", 78, "NumpadAdd", "Keypad +"},
		{"keypad.enter", 156, "NumpadEnter", "Keypad Enter"},
		{"keypad.equal", 141, "NumpadEqual", "Keypad ="},
		{"up", 200, "ArrowUp", "Up"},
		{"down", 208, "ArrowDown", "Down"},
		{"left", 203, "ArrowLeft", "Left"},
		{"right", 205, "ArrowRight", "Right"},
		{"left.alt", 56, "AltLeft", "Left Alt"},
		{"right.alt", 184, "AltRight", "Right Alt"},
		{"left.shift", 42, "ShiftLeft", "Left Shift"},
		{"right.shift", 54, "ShiftRight", "Right Shift"},
		{"left.control", 29, "ControlLeft", "Left Ctrl"},
		{"right.control", 157, "ControlRight", "Right Ctrl"},
		{"left.win", 219, "MetaLeft", "Left Super"},
		{"right.win", 220, "MetaRight", "Right Super"},
		{"menu", 221, "ContextMenu", "Menu"},
		{"space", 57, "Space", "Space"},
		{"tab", 15, "Tab", "Tab"},
		{"caps.lock", 58, "CapsLock", "Caps Lock"},
		{"enter", 28, "Enter", "Enter"},
		{"escape", 1, "Escape", "Escape"},
		{"backspace", 14, "Backspace", "Backspace"},
		{"delete", 211, "Delete", "Delete"},
		{"end", 207, "End", "End"},
		{"home", 199, "Home", "Home"},
		{"insert", 210, "Insert", "Insert"},
		{"page.down", 209, "PageDown", "Page Down"},
		{"page.up", 201, "PageUp", "Page Up"},
		{"num.lock", 69, "NumLock", "Num Lock"},
		{"scroll.lock", 70, "ScrollLock", "Scroll Lock"},
		{"pause", 197, "Pause", "Pause"},
		{"print.screen", 183, "PrintScreen", "Print Screen"},
		{"apostrophe", 40, "Quote", "'"},
		{"backslash", 43, "Backslash", "\\"},
		{"comma", 51, "Comma", ","},
		{"equal", 13, "Equal", "="},
		{"grave.accent", 41, "Backquote", "`"},
		{"left.bracket", 26, "BracketLeft", "["},
		{"minus", 12, "Minus", "-"},
		{"period", 52, "Period", "."},
		{"right.bracket", 27, "BracketRight", "]"},
		{"semicolon", 39, "Semicolon", ";"},
		{"slash", 53, "Slash", "/"},
		{"world.1", 0, "", "World 1"},
		{"world.2", 86, "IntlBackslash", "World 2"},
	} {
		add("key.keyboard."+k.name, k.legacy, k.code, k.label)
	}
	// Mouse buttons: LWJGL2 wrote button n as n-100; GLFW names 0-2 and
	// numbers the rest from 4. The browser numbers middle and right the
	// other way round.
	add("key.mouse.left", -100, "Mouse0", "Left Button")
	add("key.mouse.right", -99, "Mouse2", "Right Button")
	add("key.mouse.middle", -98, "Mouse1", "Middle Button")
	for b := 3; b <= 7; b++ {
		s := strconv.Itoa(b + 1)
		add("key.mouse."+s, b-100, "Mouse"+strconv.Itoa(b), "Mouse "+s)
	}
}

func letter(c rune, legacy int) {
	s := string(c)
	add("key.keyboard."+s, legacy, "Key"+strings.ToUpper(s), strings.ToUpper(s))
}

// ByName looks a key up by its 1.13+ name.
func ByName(name string) (Key, bool) { k, ok := byName[name]; return k, ok }

// ByLegacy looks a key up by its pre-1.13 code.
func ByLegacy(code int) (Key, bool) { k, ok := byLegacy[code]; return k, ok }

// ByBrowser looks a key up by the browser's code ("KeyC", "Mouse3").
func ByBrowser(code string) (Key, bool) { k, ok := byBrowser[code]; return k, ok }

// Parse reads an options.txt key value in either format.
func Parse(v string) (Key, bool) {
	if n, err := strconv.Atoi(v); err == nil {
		if n == 0 {
			return byName[Unknown], true
		}
		return ByLegacy(n)
	}
	return ByName(v)
}

// Format writes k for options.txt; legacy selects the pre-1.13 format.
// It reports false when that format has no code for the key.
func Format(k Key, legacy bool) (string, bool) {
	if !legacy {
		return k.Name, true
	}
	if k.Name == Unknown {
		return "0", true
	}
	if k.Legacy == 0 {
		return "", false
	}
	return strconv.Itoa(k.Legacy), true
}
