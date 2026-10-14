// Package version compares version strings and checks them against the
// ranges mods declare for their dependencies: Fabric and Quilt predicates
// (">=1.2 <2", "~1.20", "1.20.x") and Maven ranges ("[1.2,2.0)") as Forge
// and NeoForge write them. It only answers when it is sure: a version or a
// range it can't read gives ok=false, so callers flag nothing on a guess.
package version

import (
	"cmp"
	"regexp"
	"strconv"
	"strings"
)

// Compare orders dotted version strings numerically part by part
// ("0.16.10" > "0.16.9"); a pre-release suffix ("-beta.1") sorts below the
// release it precedes. Non-numeric parts compare as text.
func Compare(a, b string) int {
	ab, apre, _ := strings.Cut(a, "-")
	bb, bpre, _ := strings.Cut(b, "-")
	as, bs := strings.Split(ab, "."), strings.Split(bb, ".")
	for i := range max(len(as), len(bs)) {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xi, xerr := strconv.Atoi(cmp.Or(x, "0"))
		yi, yerr := strconv.Atoi(cmp.Or(y, "0"))
		switch {
		case xerr == nil && yerr == nil:
			if c := cmp.Compare(xi, yi); c != 0 {
				return c
			}
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	switch {
	case apre == bpre:
		return 0
	case apre == "":
		return 1
	case bpre == "":
		return -1
	}
	return Compare(strings.ReplaceAll(apre, "-", "."), strings.ReplaceAll(bpre, "-", "."))
}

// Style is how a range is written.
type Style int

const (
	// Fabric predicates: space-separated terms that must all hold, with
	// alternatives (a JSON array in fabric.mod.json) joined by "||".
	Fabric Style = iota
	// Maven ranges: "[1,2)", "(,1.5]", "[1.0]", comma-separated unions. A
	// bare version is only a recommendation, so it accepts anything.
	Maven
)

// Any is the range that accepts every version.
const Any = "*"

// Unknown stands for a range that was declared but couldn't be read: it
// never gives an answer.
const Unknown = "(?)"

// plain matches a version this package is willing to judge: dotted numbers,
// an optional pre-release that starts with a letter ("-beta.2", "-pre1") and
// optional build metadata ("+mc1.20.1"), which is ignored. "1.20.1-47.2.0"
// (a Minecraft prefix) or "${version}" are not plain.
var plain = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)(?:-([A-Za-z][0-9A-Za-z.\-]*))?(?:\+[0-9A-Za-z.\-]*)?$`)

// clean is v's comparable form, or false when v isn't a plain version.
func clean(v string) (string, bool) {
	m := plain.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return "", false
	}
	if m[2] != "" {
		return m[1] + "-" + m[2], true
	}
	return m[1], true
}

// Satisfies reports whether version v is inside range r. ok is false when
// either can't be read with certainty.
func Satisfies(st Style, r, v string) (in, ok bool) {
	r = strings.TrimSpace(r)
	switch r {
	case "", Any:
		return true, true
	case Unknown:
		return false, false
	}
	if st == Maven {
		return maven(r, v)
	}
	return fabric(r, v)
}

// ---- Fabric ----

func fabric(r, v string) (bool, bool) {
	cv, vok := clean(v)
	sure := true
	for _, alt := range strings.Split(r, "||") {
		in, ok := allOf(strings.Fields(alt), cv, vok)
		if !ok {
			sure = false
			continue
		}
		if in {
			return true, true
		}
	}
	return false, sure
}

func allOf(terms []string, v string, vok bool) (bool, bool) {
	for _, t := range terms {
		in, ok := term(t, v, vok)
		if !ok {
			return false, false
		}
		if !in {
			return false, true
		}
	}
	return true, true
}

var operators = []string{">=", "<=", ">", "<", "=", "~", "^"}

func term(t, v string, vok bool) (bool, bool) {
	if t == "*" {
		return true, true
	}
	op := ""
	for _, o := range operators {
		if strings.HasPrefix(t, o) {
			op, t = o, strings.TrimSpace(t[len(o):])
			break
		}
	}
	if !vok {
		return false, false
	}
	if lo, hi, wild := wildcard(t); wild {
		if op != "" && op != "=" {
			return false, false
		}
		return Compare(v, lo) >= 0 && Compare(v, hi) < 0, true
	}
	target, ok := clean(t)
	if !ok {
		return false, false
	}
	c := Compare(v, target)
	switch op {
	case ">=":
		return c >= 0, true
	case "<=":
		return c <= 0, true
	case ">":
		return c > 0, true
	case "<":
		return c < 0, true
	case "~":
		return c >= 0 && Compare(v, bump(target, 1)) < 0, true
	case "^":
		return c >= 0 && Compare(v, bump(target, 0)) < 0, true
	}
	return c == 0, true
}

// wildcard reads "1.20.x" (or .X, .*) as [1.20, 1.21).
func wildcard(t string) (lo, hi string, ok bool) {
	parts := strings.Split(t, ".")
	last := parts[len(parts)-1]
	if len(parts) < 2 || (last != "x" && last != "X" && last != "*") {
		return "", "", false
	}
	lo = strings.Join(parts[:len(parts)-1], ".")
	if _, ok := clean(lo); !ok {
		return "", "", false
	}
	return lo, bump(lo, len(parts)-2), true
}

// bump raises part i of a version's numbers and drops the rest: bump("1.20.4",
// 1) is "1.21". With fewer parts it raises the last one.
func bump(v string, i int) string {
	core, _, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	i = min(i, len(parts)-1)
	n, _ := strconv.Atoi(parts[i])
	parts[i] = strconv.Itoa(n + 1)
	return strings.Join(parts[:i+1], ".")
}

// ---- Maven ----

var mavenRange = regexp.MustCompile(`^\s*([\[(])([^\[\]()]*)([\])])\s*(?:,|$)`)

func maven(r, v string) (bool, bool) {
	if !strings.ContainsAny(r, "[(") {
		return true, true // a bare version only recommends one
	}
	cv, vok := clean(v)
	if !vok {
		return false, false
	}
	in := false
	for rest := r; strings.TrimSpace(rest) != ""; {
		m := mavenRange.FindStringSubmatch(rest)
		if m == nil {
			return false, false
		}
		rest = rest[len(m[0]):]
		hit, ok := bounds(m[1] == "[", m[2], m[3] == "]", cv)
		if !ok {
			return false, false
		}
		in = in || hit
	}
	return in, true
}

func bounds(loIn bool, inner string, hiIn bool, v string) (bool, bool) {
	lo, hi, two := strings.Cut(inner, ",")
	lo, hi = strings.TrimSpace(lo), strings.TrimSpace(hi)
	if !two {
		exact, ok := clean(lo)
		return ok && Compare(v, exact) == 0, ok
	}
	if lo != "" {
		l, ok := clean(lo)
		if !ok {
			return false, false
		}
		if c := Compare(v, l); c < 0 || (c == 0 && !loIn) {
			return false, true
		}
	}
	if hi != "" {
		h, ok := clean(hi)
		if !ok {
			return false, false
		}
		if c := Compare(v, h); c > 0 || (c == 0 && !hiIn) {
			return false, true
		}
	}
	return true, true
}

// ---- describing ----

// Describe shortens a range for a label: ">=0.5" is "≥ 0.5", "~1.20" and
// "[1.20,1.21)" stay recognisable, alternatives are joined with "or". It
// gives "" for a range that accepts anything.
func Describe(st Style, r string) string {
	r = strings.TrimSpace(r)
	if r == "" || r == Any || r == Unknown {
		return ""
	}
	var out []string
	if st == Maven {
		if !strings.ContainsAny(r, "[(") {
			return ""
		}
		for rest := r; strings.TrimSpace(rest) != ""; {
			m := mavenRange.FindStringSubmatch(rest)
			if m == nil {
				return r
			}
			rest = rest[len(m[0]):]
			out = append(out, describeMaven(m[1] == "[", m[2], m[3] == "]"))
		}
		return strings.Join(out, " or ")
	}
	for _, alt := range strings.Split(r, "||") {
		var terms []string
		for _, t := range strings.Fields(alt) {
			terms = append(terms, describeTerm(t))
		}
		out = append(out, strings.Join(terms, " "))
	}
	return strings.Join(out, " or ")
}

func describeTerm(t string) string {
	for _, p := range [][2]string{{">=", "≥ "}, {"<=", "≤ "}, {">", "> "}, {"<", "< "}, {"=", ""}} {
		if rest, ok := strings.CutPrefix(t, p[0]); ok {
			return p[1] + rest
		}
	}
	if rest, ok := strings.CutPrefix(t, "~"); ok {
		parts := strings.Split(rest, ".")
		return strings.Join(parts[:min(2, len(parts))], ".") + ".x"
	}
	if rest, ok := strings.CutPrefix(t, "^"); ok {
		major, _, _ := strings.Cut(rest, ".")
		return major + ".x"
	}
	return t
}

func describeMaven(loIn bool, inner string, hiIn bool) string {
	lo, hi, two := strings.Cut(inner, ",")
	lo, hi = strings.TrimSpace(lo), strings.TrimSpace(hi)
	if !two {
		return lo
	}
	var parts []string
	if lo != "" {
		parts = append(parts, map[bool]string{true: "≥ ", false: "> "}[loIn]+lo)
	}
	if hi != "" {
		parts = append(parts, map[bool]string{true: "≤ ", false: "< "}[hiIn]+hi)
	}
	return strings.Join(parts, ", ")
}
