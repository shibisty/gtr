package semver

import "testing"

// The parsers take user input (gtr.json, the command line, git tags): they
// must never panic, and what they print must parse back to the same thing.

var samples = []string{"0.0.0", "0.1.0", "0.1.9", "0.2.0", "1.0.0", "1.2.3", "1.2.4-rc.1", "1.26.9", "1.27rc1", "2.0.0", "2.0.0-0", "10.0.0"}

func FuzzParse(f *testing.F) {
	for _, s := range append(samples, "v1.2.3", "go1.26.5", "1", "1.2", "1.2.3-beta.2", "", "x", "1..2", "01.2.3", "1.2.3-") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := Parse(s)
		if err != nil {
			return
		}
		back, err := Parse(v.String())
		if err != nil || Compare(v, back) != 0 || back.String() != v.String() {
			t.Fatalf("%q → %q → %v %v", s, v.String(), back, err)
		}
	})
}

func FuzzParseRange(f *testing.F) {
	for _, s := range []string{"*", "", "^1.2", "~1.2.3", "1.2.5", ">=1.0 <2.0", "^0.0.3", ">1.2.3-rc.1", "1.x", "^^1", "<=", ">= 1.2 < 1.4", "~0", "=1.0.0"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, err := ParseRange(s)
		if err != nil {
			return
		}
		again, err := ParseRange(r.String())
		if err != nil {
			t.Fatalf("%q prints as %q, which does not parse: %v", s, r.String(), err)
		}
		for _, sv := range samples {
			v := MustParse(sv)
			if r.Match(v) != again.Match(v) {
				t.Fatalf("%q and its printed form %q disagree on %s", s, r.String(), sv)
			}
		}
		r.Best(samples) // must not panic
	})
}

func FuzzCompare(f *testing.F) {
	f.Add("1.2.3", "1.2.3-rc.1")
	f.Add("1.27rc1", "1.27.0")
	f.Add("1.0.0-alpha.1", "1.0.0-alpha.beta")
	f.Fuzz(func(t *testing.T, a, b string) {
		va, err1 := Parse(a)
		vb, err2 := Parse(b)
		if err1 != nil || err2 != nil {
			return
		}
		if Compare(va, vb) != -Compare(vb, va) {
			t.Fatalf("Compare(%s, %s) is not antisymmetric", a, b)
		}
	})
}
