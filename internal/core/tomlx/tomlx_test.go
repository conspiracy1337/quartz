package tomlx

import (
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// corpus covers what mods.toml and packwiz files use plus the TOML corners
// the decoder and encoder have to get right.
var corpus = []string{
	``,
	"# only a comment\n",
	`modLoader="javafml"
loaderVersion="[47,)"
license="MIT"
issueTrackerURL="https://example.com/issues"
[[mods]]
modId="examplemod"
version="${file.jarVersion}"
displayName="Example Mod"
authors="Someone, Someone Else"
description='''
A multi-line
description.
'''
logoFile="logo.png"
[[dependencies.examplemod]]
    modId="forge"
    mandatory=true
    versionRange="[47,)"
    ordering="NONE"
    side="BOTH"
[[dependencies.examplemod]]
    modId="minecraft"
    type="required"
    versionRange="[1.20.1,1.21)"
`,
	`name = "Sodium"
filename = "sodium-fabric-0.5.8+mc1.20.4.jar"
side = "client"
x-prismlauncher-loaders = [ "fabric", "quilt" ]
x-prismlauncher-mc-versions = [ "1.20.4" ]
x-prismlauncher-release-type = "release"

[download]
hash = "a1b2c3"
hash-format = "sha512"
mode = "url"
url = "https://cdn.modrinth.com/data/AANobbMI/versions/x/sodium.jar"

[update]
[update.modrinth]
mod-id = "AANobbMI"
version = "4GyXKCLd"
[update.curseforge]
file-id = 5000000
project-id = 394468
`,
	`[[x-prismlauncher-dependencies]]
addonId = "P7dR8mSH"
type = "required"
[[x-prismlauncher-dependencies]]
addonId = "abc"
type = "optional"
version = "1.0"
`,
	`a.b.c = 1
a.d = "x"
"quoted key" = true
'literal key' = false
"" = "empty key"
"a.b" = "dotted in quotes"
inline = { x = 1, y.z = [1, 2.5, "s", { k = "v" }], e = {} }
empty = []
nested = [[1, 2], ["a"], []]
tables = [{ a = 1 }, { b = 2 }]
mixed = [1, { a = 1 }]
`,
	`ints = [0, +1, -1, 1_000, 0xDEAD_beef, 0o755, 0b1101, 9223372036854775807, -9223372036854775808]
floats = [0.0, -0.0, +1.5, 1e10, 1E-10, 6.626e-34, 1_000.5, 3.0e+2, 0.1, 123456789012345678901234567890.0]
specials = [inf, +inf, -inf, nan, +nan, -nan]
bools = [true, false]
`,
	`odt1 = 1979-05-27T07:32:00Z
odt2 = 1979-05-27T00:32:00-07:00
odt3 = 1979-05-27T00:32:00.999999+07:00
odt4 = 1979-05-27 07:32:00Z
odt5 = 1979-05-27t07:32:00z
odt6 = 1979-05-27T07:32:00+00:00
ldt1 = 1979-05-27T07:32:00
ldt2 = 1979-05-27T00:32:00.999999
ldt3 = 1979-05-27T07:32
ld1 = 1979-05-27
lt1 = 07:32:00
lt2 = 00:32:00.999999
lt3 = 00:32:00.100
lt4 = 07:32
lt5 = 07:32:00.1234567891
`,
	`basic = "tab\there \"quoted\" back\\slash \u00e9 \U0001F600"
literal = 'C:\Users\nodejs\templates'
ml = """
Roses are red
Violets are blue"""
ml2 = """\
       The quick brown \
       fox."""
mll = '''
The first newline is
trimmed in raw strings.
'''
ctrl = "a\u0001b\u007fc"
apos = "it's"
`,
	`[a]
x = 1
[a.b]
y = 2
[c]
[d.e.f]
g = 3
[[h]]
i = 1
[[h]]
i = 2
[h.j]
k = 3
[[h.l]]
m = 4
`,
	"key = \"value\" # trailing comment\r\nother = 1\r\n",
	`[fruit]
apple.color = "red"
apple.taste.sweet = true
[fruit.apple.texture]
smooth = true
`,
	"x = [\n  1,\n  # comment\n  2,\n]\n",
	"unicode = \"日本語\"\n\"ключ\" = 'значение'\n",
	// Invalid documents: both must refuse them.
	"a = 1\na = 2\n",
	"[a]\n[a]\n",
	"a = 1\n[a]\n",
	"[[a]]\n[a]\n",
	"a.b = 1\n[a]\n",
	"a = {x = 1}\na.y = 2\n",
	"a = [1,]\nb = \n",
	"key = = oops\n",
	"x = 1979-02-30\n",
	"x = 0x8000000000000000\n",
	"x = 99999999999999999999\n",
	"x = 25:00:00\n",
	"x = 1e400\n",
	"[[a]]\nb = 1\n[a.b]\n",
	"x = \"unterminated\n",
	"a.b = 1\na.b.c = 2\n",
	"[a.b]\n[a]\nb = 1\n",
}

// normalize turns go-toml's date and time types into this package's.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalize(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	case toml.LocalDate:
		return LocalDate{x.Year, x.Month, x.Day}
	case toml.LocalTime:
		return LocalTime{x.Hour, x.Minute, x.Second, x.Nanosecond, x.Precision}
	case toml.LocalDateTime:
		return LocalDateTime{normalize(x.LocalDate).(LocalDate), normalize(x.LocalTime).(LocalTime)}
	}
	return v
}

// same compares decoded trees, treating NaN as equal to NaN and time.Time by
// instant and offset.
func same(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !same(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !same(x[i], y[i]) {
				return false
			}
		}
		return true
	case float64:
		y, ok := b.(float64)
		return ok && (x == y && math.Signbit(x) == math.Signbit(y) || math.IsNaN(x) && math.IsNaN(y))
	case time.Time:
		y, ok := b.(time.Time)
		_, xo := x.Zone()
		_, yo := y.Zone()
		return ok && x.Equal(y) && xo == yo
	}
	return reflect.DeepEqual(a, b)
}

// check compares Decode and Encode with go-toml's Unmarshal and Marshal.
func check(t *testing.T, doc string) {
	t.Helper()
	got, gotErr := Decode([]byte(doc))
	var want map[string]any
	wantErr := toml.Unmarshal([]byte(doc), &want)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("%q: error %v, go-toml %v", doc, gotErr, wantErr)
	}
	if gotErr != nil {
		return
	}
	if !same(got, normalize(want)) {
		t.Fatalf("%q:\n got %#v\nwant %#v", doc, got, normalize(want))
	}
	enc, err := Encode(got)
	if err != nil {
		t.Fatalf("%q: encode: %v", doc, err)
	}
	wantEnc, err := toml.Marshal(want)
	if err != nil {
		t.Fatalf("%q: go-toml marshal: %v", doc, err)
	}
	if string(enc) != string(wantEnc) {
		t.Fatalf("%q: encoded\n%s\ngo-toml\n%s", doc, enc, wantEnc)
	}
}

func TestMatchesGoTOML(t *testing.T) {
	for _, doc := range corpus {
		check(t, doc)
	}
}

// TestMutationsMatchGoTOML edits the corpus at random and checks both
// libraries still agree, on errors above all.
func TestMutationsMatchGoTOML(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	pieces := []string{"=", "[", "]", "[[", "]]", ".", "\"", "'", "\n", " ", "#", "{", "}", ",", "a", "1", "-", ":", "T", "e", "_", "x", "0"}
	for _, doc := range corpus {
		if doc == "" {
			continue
		}
		for range 300 {
			b := []byte(doc)
			for range 1 + rng.IntN(3) {
				i := rng.IntN(len(b))
				switch rng.IntN(3) {
				case 0:
					b = append(b[:i], b[i+1:]...)
				case 1:
					b = append(b[:i], append([]byte(pieces[rng.IntN(len(pieces))]), b[i:]...)...)
				default:
					b[i] = pieces[rng.IntN(len(pieces))][0]
				}
				if len(b) == 0 {
					break
				}
			}
			check(t, string(b))
		}
	}
}

// TestEncodeWriteTypes covers the Go types packwiz writes that decoding
// never produces.
func TestEncodeWriteTypes(t *testing.T) {
	doc := map[string]any{
		"name":     "Mod",
		"loaders":  []string{"fabric", "quilt"},
		"none":     []string(nil),
		"deps":     []map[string]any{{"addonId": "a", "type": "required"}, {"addonId": "b'c", "type": "optional", "version": "1.0"}},
		"nodeps":   []map[string]any{},
		"update":   map[string]any{"curseforge": map[string]any{"project-id": int64(1), "file-id": 2}, "empty": map[string]any{}},
		"nilmap":   map[string]any(nil),
		"skip":     nil,
		"key.dot":  "q",
		"multi\nl": "line\nbreak",
		"bad":      "\xff",
	}
	got, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	want, err := toml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("encoded\n%s\ngo-toml\n%s", got, want)
	}
	if strings.Contains(string(got), "skip") {
		t.Fatal("nil values are left out")
	}
}
