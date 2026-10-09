package semver

import "testing"

func TestParseAndCompare(t *testing.T) {
	order := []string{"0.1.0", "0.1.1-rc.1", "0.1.1", "1.20", "1.20.14", "1.27rc1", "1.27rc2", "1.27rc10", "1.27.0", "go1.27.2", "v2.0.0"}
	for i := 1; i < len(order); i++ {
		a, b := MustParse(order[i-1]), MustParse(order[i])
		if Compare(a, b) >= 0 || Compare(b, a) <= 0 {
			t.Errorf("%s must be < %s", order[i-1], order[i])
		}
	}
	if Compare(MustParse("1.2"), MustParse("1.2.0")) != 0 {
		t.Error("1.2 == 1.2.0")
	}
	if MustParse("v1.2.3-rc.1").String() != "1.2.3-rc.1" || MustParse("1.27rc1").Pre != "rc1" {
		t.Error("String/Pre")
	}
	for _, bad := range []string{"", "x", "1.2.3.4", "1..2", "-1"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) must fail", bad)
		}
	}
}

func TestRange(t *testing.T) {
	cases := []struct {
		rng     string
		yes, no []string
	}{
		{"*", []string{"0.0.1", "1.26.9"}, []string{"1.27rc1"}},
		{"1.26.5", []string{"1.26.5"}, []string{"1.26.6", "1.26.4"}},
		{"1.26", []string{"1.26.0", "1.26.9"}, []string{"1.27.0", "1.25.9", "1.27rc1"}},
		{"1", []string{"1.0.0", "1.99.0"}, []string{"2.0.0", "2.0.0-rc.1"}},
		{"^1.22", []string{"1.22.0", "1.27.2"}, []string{"1.21.9", "2.0.0"}},
		{"^0.2", []string{"0.2.0", "0.2.9"}, []string{"0.3.0", "0.1.9"}},
		{"^0.2.3", []string{"0.2.3", "0.2.9"}, []string{"0.3.0", "0.2.2"}},
		{"^0.0.3", []string{"0.0.3"}, []string{"0.0.4"}},
		{"~1.26", []string{"1.26.0", "1.26.9"}, []string{"1.27.0"}},
		{"~1.2.3", []string{"1.2.3", "1.2.9"}, []string{"1.3.0", "1.2.2"}},
		{">=1.26", []string{"1.26.0", "2.1.0"}, []string{"1.25.9", "1.27rc1"}},
		{">=1.0 <2.0", []string{"1.0.0", "1.9.9"}, []string{"2.0.0", "0.9.0"}},
		{">= 1.22 < 1.24", []string{"1.23.5"}, []string{"1.24.0"}},
		{">1.2 <=1.4", []string{"1.3.0", "1.4.9"}, []string{"1.2.9", "1.5.0"}},
		{">1.2.0 <=1.4.0", []string{"1.2.1", "1.4.0"}, []string{"1.2.0", "1.4.1"}},
		{"=1.2.3", []string{"1.2.3"}, []string{"1.2.4"}},
		{"<=1.26", []string{"1.26.0", "1.26.9", "1.25.1"}, []string{"1.27.0", "1.27rc1"}},
		{"<=1", []string{"1.99.0"}, []string{"2.0.0"}},
		{">1.26", []string{"1.27.0"}, []string{"1.26.9"}},
		{">1", []string{"2.0.0"}, []string{"1.99.0"}},
		{"=1.26", []string{"1.26.0", "1.26.9"}, []string{"1.27.0", "1.25.9"}},
		{"<1.26", []string{"1.25.9"}, []string{"1.26.0"}},
		{"1.27rc1", []string{"1.27rc1"}, []string{"1.27.0", "1.27rc2"}},
		{">=1.27rc1", []string{"1.27rc1", "1.27rc2", "1.27.0"}, []string{"1.28rc1"}},
	}
	for _, c := range cases {
		r, err := ParseRange(c.rng)
		if err != nil {
			t.Fatalf("ParseRange(%q): %v", c.rng, err)
		}
		for _, v := range c.yes {
			if !r.Match(MustParse(v)) {
				t.Errorf("%q must match %s", c.rng, v)
			}
		}
		for _, v := range c.no {
			if r.Match(MustParse(v)) {
				t.Errorf("%q must not match %s", c.rng, v)
			}
		}
	}
	for _, bad := range []string{"^", ">=", ">=x", "1.2.3.4", "~>1", "<<1.26", "=<1.2", "=>1.2", ">==1"} {
		if _, err := ParseRange(bad); err == nil {
			t.Errorf("ParseRange(%q) must fail", bad)
		}
	}
	if r, _ := ParseRange("1.2.3"); !r.Exact() || r.String() != "1.2.3" {
		t.Error("Exact/String")
	}
	if r, _ := ParseRange(""); r.Exact() || r.String() != "*" {
		t.Error("empty range")
	}
}

func TestBest(t *testing.T) {
	r, _ := ParseRange("^1.22")
	if got, ok := r.Best([]string{"1.21.9", "1.26.9", "1.27.2", "1.28rc1", "junk", "2.0.0"}); !ok || got != "1.27.2" {
		t.Fatalf("Best = %q %v", got, ok)
	}
	if _, ok := r.Best([]string{"1.21.0"}); ok {
		t.Fatal("no match expected")
	}
}
