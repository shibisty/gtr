// Package semver parses versions and npm-style ranges (ADR-0004).
//
// It understands both SemVer ("0.2.1", "v1.0.0-rc.1") and Go release names
// ("1.26.9", "1.20", "1.27rc1"), so one range syntax selects Go and
// gtr-manager versions alike.
package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a parsed version. Missing minor/patch parts are zero.
type Version struct {
	Major, Minor, Patch int
	Pre                 string // pre-release ("rc.1", "rc1", "beta2"); empty for stable
}

// versionRe: pre-release either after "-" (SemVer) or glued to the number
// and starting with a letter (Go: "1.27rc1").
var versionRe = regexp.MustCompile(`^v?(?:go)?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([0-9A-Za-z][0-9A-Za-z.-]*)|([A-Za-z][0-9A-Za-z.-]*))?$`)

// Parse parses a full or partial version. "go1.27rc1" and "v1.2.3" are accepted.
func Parse(s string) (Version, error) {
	v, _, err := parse(strings.TrimSpace(s))
	return v, err
}

// MustParse is Parse that panics on error (for tests and constants).
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// parse also returns how many numeric parts were given (1–3).
func parse(s string) (Version, int, error) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, 0, fmt.Errorf("invalid version %q", s)
	}
	var v Version
	parts := 1
	v.Major, _ = strconv.Atoi(m[1])
	if m[2] != "" {
		v.Minor, _ = strconv.Atoi(m[2])
		parts = 2
	}
	if m[3] != "" {
		v.Patch, _ = strconv.Atoi(m[3])
		parts = 3
	}
	v.Pre = m[4] + m[5]
	return v, parts, nil
}

// String formats the version as major.minor.patch[-pre].
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare returns -1, 0 or +1. A pre-release sorts before its release.
func Compare(a, b Version) int {
	for _, d := range [3]int{a.Major - b.Major, a.Minor - b.Minor, a.Patch - b.Patch} {
		if d != 0 {
			if d < 0 {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	default:
		return comparePre(a.Pre, b.Pre)
	}
}

// comparePre compares pre-release identifiers, numerically where possible
// ("rc10" > "rc9", "rc.10" > "rc.9").
func comparePre(a, b string) int {
	split := regexp.MustCompile(`\d+|[^\d.]+`)
	pa, pb := split.FindAllString(a, -1), split.FindAllString(b, -1)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, ex := strconv.Atoi(pa[i])
		y, ey := strconv.Atoi(pb[i])
		switch {
		case ex == nil && ey == nil && x != y:
			if x < y {
				return -1
			}
			return 1
		case ex != nil || ey != nil:
			if c := strings.Compare(pa[i], pb[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(pa) < len(pb):
		return -1
	case len(pa) > len(pb):
		return 1
	}
	return 0
}

type comparator struct {
	op string // ">=", ">", "<=", "<", "="
	v  Version
}

func (c comparator) match(v Version) bool {
	cmp := Compare(v, c.v)
	switch c.op {
	case ">=":
		return cmp >= 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case "<":
		return cmp < 0
	default:
		return cmp == 0
	}
}

// Range is a set of comparators that must all match.
type Range struct {
	raw   string
	cmps  []comparator
	pre   []Version // versions named in the range; their pre-releases may match
	exact bool      // the range is a single exact version
}

// ParseRange parses "*", "1.2.3", "1.2", "1", "^1.2", "~1.2.3", "v1.2.3-rc.1"
// and comparator lists such as ">=1.0 <2.0".
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(s)
	r := Range{raw: s}
	if s == "" || s == "*" || s == "x" || s == "latest" {
		return r, nil
	}
	if strings.ContainsAny(s[:1], "<>=") {
		for _, f := range splitComparators(s) {
			op := f[:len(f)-len(strings.TrimLeft(f, "<>="))]
			v, parts, err := parse(strings.TrimSpace(f[len(op):]))
			if err != nil || !validOp[op] {
				return Range{}, fmt.Errorf("invalid range %q", s)
			}
			if v.Pre != "" {
				r.pre = append(r.pre, v)
				parts = 3 // a pre-release is always a full version
			}
			r.cmps = append(r.cmps, expand(op, v, parts)...)
		}
		return r, nil
	}

	prefix := ""
	if s[0] == '^' || s[0] == '~' {
		prefix, s = s[:1], s[1:]
	}
	v, parts, err := parse(s)
	if err != nil {
		return Range{}, fmt.Errorf("invalid range %q", r.raw)
	}
	if v.Pre != "" {
		r.pre = append(r.pre, v)
	}
	lo := comparator{">=", v}
	var hi Version
	switch {
	case prefix == "" && (parts == 3 || v.Pre != ""):
		r.cmps, r.exact = []comparator{{"=", v}}, true
		return r, nil
	case prefix == "^":
		switch {
		case v.Major > 0 || parts == 1:
			hi = Version{Major: v.Major + 1}
		case v.Minor > 0 || parts == 2:
			hi = Version{Minor: v.Minor + 1}
		default:
			hi = Version{Patch: v.Patch + 1}
		}
	case prefix == "~" && parts >= 2, prefix == "" && parts == 2:
		hi = Version{Major: v.Major, Minor: v.Minor + 1}
	default: // "1", "~1"
		hi = Version{Major: v.Major + 1}
	}
	hi.Pre = "0" // excludes pre-releases of the upper bound: <2.0.0-0
	r.cmps = []comparator{lo, {"<", hi}}
	return r, nil
}

var validOp = map[string]bool{">=": true, ">": true, "<=": true, "<": true, "=": true}

// expand turns a comparator with a partial version into full-version
// comparators, as npm does: "<=1.26" is "<1.27.0-0" (all of 1.26.x),
// ">1.26" is ">=1.27.0", "=1.26" is the range 1.26.x.
func expand(op string, v Version, parts int) []comparator {
	if parts == 3 {
		return []comparator{{op, v}}
	}
	next := Version{Major: v.Major + 1, Pre: "0"}
	if parts == 2 {
		next = Version{Major: v.Major, Minor: v.Minor + 1, Pre: "0"}
	}
	switch op {
	case "<=":
		return []comparator{{"<", next}}
	case ">":
		next.Pre = ""
		return []comparator{{">=", next}}
	case "=":
		return []comparator{{">=", v}, {"<", next}}
	default: // ">=", "<"
		return []comparator{{op, v}}
	}
}

// splitComparators splits ">=1.0 <2" and ">= 1.0 < 2" into [">=1.0", "<2"].
func splitComparators(s string) []string {
	var out []string
	fields := strings.Fields(s)
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.Trim(f, "<>=") == "" && i+1 < len(fields) {
			f += fields[i+1]
			i++
		}
		out = append(out, f)
	}
	return out
}

// Match reports whether v satisfies the range. Pre-releases match only if
// the range names a pre-release of the same major.minor.patch.
func (r Range) Match(v Version) bool {
	if v.Pre != "" {
		allowed := false
		for _, p := range r.pre {
			if p.Major == v.Major && p.Minor == v.Minor && p.Patch == v.Patch {
				allowed = true
			}
		}
		if !allowed {
			return false
		}
	}
	for _, c := range r.cmps {
		if !c.match(v) {
			return false
		}
	}
	return true
}

// Exact reports whether the range names one exact version.
func (r Range) Exact() bool { return r.exact }

// String returns the range as written.
func (r Range) String() string {
	if r.raw == "" {
		return "*"
	}
	return r.raw
}

// Best returns the highest version from candidates that satisfies r.
func (r Range) Best(candidates []string) (string, bool) {
	best, found := "", false
	var bv Version
	for _, c := range candidates {
		v, err := Parse(c)
		if err != nil || !r.Match(v) {
			continue
		}
		if !found || Compare(v, bv) > 0 {
			best, bv, found = c, v, true
		}
	}
	return best, found
}
