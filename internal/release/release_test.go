package release

import (
	"errors"
	"testing"

	"gtr/internal/semver"
)

func TestSelect(t *testing.T) {
	rels := []Release{{Tag: "v0.1.0"}, {Tag: "v0.2.0"}, {Tag: "0.2.5"}, {Tag: "v0.2.6-rc.1", Prerelease: true},
		{Tag: "v0.3.0-beta", Prerelease: true}, {Tag: "nightly"}}
	for req, want := range map[string]string{"*": "0.2.5", "^0.2": "0.2.5", "0.1": "0.1.0", "0.2.6-rc.1": "v0.2.6-rc.1"} {
		r, err := Select(rels, mustRange(req))
		if err != nil || r.Tag != want && r.Version() != want {
			t.Errorf("Select(%s) = %v %v, want %s", req, r, err, want)
		}
	}
	if _, err := Select(rels, mustRange("^1")); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("got %v", err)
	}
}

func TestNames(t *testing.T) {
	s := &Source{App: "gtr-manager", GOOS: "windows", GOARCH: "amd64"}
	if s.AssetName("0.2.0") != "gtr-manager_0.2.0_windows_amd64.zip" || s.BinaryName() != "gtr-manager.exe" {
		t.Fatal(s.AssetName("0.2.0"), s.BinaryName())
	}
	s = &Source{App: "gtr", GOOS: "darwin", GOARCH: "arm64"}
	if s.AssetName("1.0.0") != "gtr_1.0.0_darwin_arm64.tar.gz" || s.BinaryName() != "gtr" {
		t.Fatal(s.AssetName("1.0.0"))
	}
}

func mustRange(s string) semver.Range {
	r, err := semver.ParseRange(s)
	if err != nil {
		panic(err)
	}
	return r
}

func TestSameHost(t *testing.T) {
	for _, c := range []struct {
		url, api string
		want     bool
	}{
		{"https://api.github.com/repos/x/y/releases", "https://api.github.com", true},
		{"https://API.github.com/x", "https://api.github.com/", true},
		{"https://api.github.com.evil.com/x", "https://api.github.com", false},
		{"http://api.github.com/x", "https://api.github.com", false},
		{"https://objects.githubusercontent.com/x", "https://api.github.com", false},
	} {
		if got := sameHost(c.url, c.api); got != c.want {
			t.Errorf("sameHost(%s, %s) = %v", c.url, c.api, got)
		}
	}
}
