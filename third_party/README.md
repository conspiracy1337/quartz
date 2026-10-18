# Patched dependencies

Copies of modules that Wails pulls in, swapped in with `replace` in go.mod
because the originals cost far more binary size than the part we use (the
release binary must stay under 20 MB). Each keeps its upstream license. When
Wails is bumped, check that the copy still covers what Wails calls: a missing
function fails the build, it never fails silently.

## go-toast (git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3)

Wails' Windows frontend sends notifications through it. Upstream built the
toast XML and its PowerShell fallback with `text/template`, which linked the
template engine into every Windows build (~230 KB). The `tmpl` package is
gone: `xml.go` writes the same elements and attributes directly
(`xml_test.go` compares it with the original template), and
`wintoast/script.go` writes the fallback script. Both also close holes the
templates had: attribute values are escaped, `]]>` can't end a CDATA
section, and the script receives the app ID and XML as base64 instead of
inside a double-quoted here-string where PowerShell ran `$(...)` from the
notification text. Everything else is upstream, unchanged (`examples/` left
out). Wails reaches `wintoast.registerClassFactory` by `//go:linkname`, so
keep that function's name and signature.
