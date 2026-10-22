// Package assets holds the launcher's own pictures and fonts in one place:
// the app icon (appicon.png: Wails packages it, staged into build/ by
// tools/release, and it is embedded here as the window and app-menu icon),
// and the logo and fonts the frontend bundles.
package assets

import _ "embed"

// Icon is the launcher's icon (1024 px PNG), the one appicon.png.
//
//go:embed appicon.png
var Icon []byte

// ServerIcon is what a hosted server shows in server lists when the player
// picked no picture (64 px PNG, as Minecraft wants).
//
//go:embed server-icon.png
var ServerIcon []byte
