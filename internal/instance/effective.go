package instance

import (
	"errors"

	"quartz/internal/settings"
)

// ErrJavaNotChosen fails a launch when Java isn't picked automatically and
// neither the instance nor Settings names one: nothing is downloaded silently.
var ErrJavaNotChosen = errors.New("instance: pick Java automatically is off. Choose a runtime in Settings, Java")

// Effective is an instance's settings with the global ones filled in: the one
// place that decides what a launch actually uses (Prism's override model, as
// the instance settings page shows it).
type Effective struct {
	JavaPath    string // "" picks Java automatically
	JavaUnset   bool   // automatic picking is off and no runtime is chosen
	MemoryMinMB int
	MemoryMaxMB int
	JVMArgs     string
	Width       int // 0 keeps the game's default size
	Height      int
	Fullscreen  bool
	OnGameStart string // keep-open | hide | close
	OpenInto    Into
	Commands    CommandSettings
	Performance PerformanceSettings
	Account     string // "" plays as the default account
}

// minHeapMB keeps an overridden minimum heap from being absurdly small.
const minHeapMB = 256

// Resolve layers in's settings over the globals.
func (in Instance) Resolve(g settings.Settings) Effective {
	e := Effective{
		MemoryMinMB: g.Java.MemoryMinMB, MemoryMaxMB: g.Java.MemoryMaxMB, JVMArgs: g.Java.JVMArgs,
		OnGameStart: settings.Resolve(nonEmpty(in.Settings.Window.OnGameStart), g.General.OnGameStart),
		Width:       in.Settings.Window.Width, Height: in.Settings.Window.Height, Fullscreen: in.Settings.Window.Fullscreen,
		OpenInto: in.Settings.Window.OpenInto,
		Commands: in.Settings.Commands, Performance: in.Settings.Performance, Account: in.Settings.Account,
	}
	if j := in.Settings.Java; j.Override {
		e.JavaPath, e.JVMArgs = j.JavaPath, j.JVMArgs
		if j.MemoryMaxMB > 0 {
			e.MemoryMinMB, e.MemoryMaxMB = max(j.MemoryMinMB, minHeapMB), j.MemoryMaxMB
		}
	}
	if e.JavaPath == "" && !g.Java.AutoPick {
		e.JavaPath = g.Java.DefaultPath
		e.JavaUnset = e.JavaPath == ""
	}
	e.MemoryMinMB = min(e.MemoryMinMB, e.MemoryMaxMB)
	return e
}

// nonEmpty is nil for "", so settings.Resolve falls back to the global value.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
