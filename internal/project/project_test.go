package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFind(t *testing.T) {
	root := t.TempDir()
	if p, err := Find(root); err != nil || p != nil {
		t.Fatalf("no gtr.json: %v %v", p, err)
	}
	os.WriteFile(filepath.Join(root, FileName), []byte(`{"engines":{"go":">=1.26","gtr":"^0.2"},"engine":"1.20"}`), 0o644)
	deep := filepath.Join(root, "a", "b")
	os.MkdirAll(deep, 0o755)
	p, err := Find(deep)
	if err != nil || p.Dir != root || p.Go != ">=1.26" || p.Gtr != "^0.2" {
		t.Fatalf("%+v %v", p, err)
	}
	os.WriteFile(filepath.Join(root, FileName), []byte(`{"engine":">=1.22"}`), 0o644)
	if p, _ := Find(deep); p.Go != ">=1.22" {
		t.Fatalf("legacy engine: %+v", p)
	}
	os.WriteFile(filepath.Join(root, FileName), []byte(`{`), 0o644)
	if _, err := Find(deep); err == nil {
		t.Fatal("broken gtr.json must be an error")
	}
	os.Remove(filepath.Join(root, FileName))
	os.Mkdir(filepath.Join(deep, FileName), 0o755) // a directory named gtr.json is ignored
	if p, err := Find(deep); err != nil || p != nil {
		t.Fatalf("dir named gtr.json: %v %v", p, err)
	}
}
