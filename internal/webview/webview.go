// Package webview keeps the webview's own storage and cache inside the data
// folder, so the launcher leaves no folders of its own elsewhere.
//
// On Linux, WebKitGTK puts them in glib's user data and cache folders under
// the program name (~/.local/share/<name>, ~/.cache/<name>), and Wails offers
// no option to change that, so Isolate points glib at dir. On Windows the
// Wails option WebviewUserDataPath does the same; on macOS WKWebView's
// folders can't be moved.
package webview
