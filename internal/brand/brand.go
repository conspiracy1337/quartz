// Package brand is the single place that names the product. Nothing else in the
// codebase may hard-code the launcher's name, folder, scheme or IDs.
package brand

const (
	// Name is the full product name shown in the UI and window title.
	Name = "Quartz Client"
	// ShortName is used where space is tight (sidebar brand, tray).
	ShortName = "Quartz"
	// Slug names the binary and the link scheme.
	Slug = "quartz"
	// Scheme is the launcher's own URL scheme (quartz://…).
	Scheme = Slug
	// DirName is the data folder name inside the OS data directory.
	DirName = "Quartz"
	// EnvDir overrides the data folder location.
	EnvDir = "QUARTZ_DIR"
	// PortableMarker, placed next to the executable, switches on portable mode.
	PortableMarker = "portable.txt"
	// AppID identifies the app for the single-instance lock and OS integration.
	AppID = "dev.quartzclient.launcher"
)

// Version is set at build time with -ldflags "-X quartz/internal/brand.Version=…".
var Version = "0.0.0-dev"
