package api

import wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

// The native dialogs are the only way a path reaches Go from a click: the UI
// never sends one. They are variables so a development build (-tags e2e) can
// answer them from a folder when the window is driven by a test.
var (
	openFileDialog = wruntime.OpenFileDialog
	openDirDialog  = wruntime.OpenDirectoryDialog
	saveFileDialog = wruntime.SaveFileDialog
)
