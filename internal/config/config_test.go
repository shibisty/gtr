package config

import (
	"os"
	"testing"

	"gtr/internal/home"
)

func TestLoadSave(t *testing.T) {
	l := home.Layout{Root: t.TempDir() + "/new"}
	c, err := Load(l)
	if err != nil || c.Go != "" {
		t.Fatalf("missing file: %+v %v", c, err)
	}
	c = &Config{Go: "1.26.9", Gtr: "0.2.0", Links: map[string]string{"gtr": "/x"}}
	if err := Save(l, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(l)
	if err != nil || got.Go != "1.26.9" || got.Links["gtr"] != "/x" {
		t.Fatalf("%+v %v", got, err)
	}
	os.WriteFile(Path(l), []byte("{"), 0o644)
	if _, err := Load(l); err == nil {
		t.Fatal("broken config must be an error")
	}
}
