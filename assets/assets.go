// Package assets holds the launcher's own pictures and fonts in one place:
// the OS icon source Wails packages (appicon.png, staged into build/ by
// tools/release), the window and app-menu icon embedded here, and the logo
// and fonts the frontend bundles.
package assets

import _ "embed"

// Icon is the launcher's icon (512 px PNG).
//
//go:embed icon.png
var Icon []byte

// ServerIcon is what a hosted server shows in server lists when the player
// picked no picture (64 px PNG, as Minecraft wants).
//
//go:embed server-icon.png
var ServerIcon []byte
