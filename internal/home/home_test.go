package home

import (
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	t.Setenv("GTR_HOME", "rel/dir")
	d, err := Dir()
	if err != nil || !filepath.IsAbs(d) || filepath.Base(d) != "dir" {
		t.Fatalf("GTR_HOME: %q %v", d, err)
	}
	t.Setenv("GTR_HOME", "")
	d, err = Dir()
	if err != nil || filepath.Base(d) != ".gtr" {
		t.Fatalf("default: %q %v", d, err)
	}
	l := Layout{Root: "/r"}
	for got, want := range map[string]string{
		l.GoVersion("1.26.5"): "/r/go/1.26.5", l.ManagerDir(): "/r/gtr",
		l.Downloads(): "/r/cache/downloads", l.Tmp(): "/r/tmp",
	} {
		if got != filepath.FromSlash(want) {
			t.Errorf("%s != %s", got, want)
		}
	}
}
