// SPDX-License-Identifier: MIT

package jsonstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rec struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestLoad_MissingFileIsFirstBoot(t *testing.T) {
	var out []rec
	if err := Load(filepath.Join(t.TempDir(), "absent.json"), &out); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if out != nil {
		t.Fatalf("out should be untouched, got %v", out)
	}
}

func TestLoad_EmptyAndWhitespaceAndBOM(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]byte{
		"empty.json": {},
		"space.json": []byte("  \n\t"),
		"bom.json":   {0xEF, 0xBB, 0xBF},
	}
	for name, content := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, content, 0o644); err != nil {
			t.Fatal(err)
		}
		var out []rec
		if err := Load(p, &out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out != nil {
			t.Fatalf("%s: out should stay untouched", name)
		}
	}
	// BOM followed by real JSON must parse.
	p := filepath.Join(dir, "bomjson.json")
	if err := os.WriteFile(p, append([]byte{0xEF, 0xBB, 0xBF}, []byte(`[{"name":"a","n":1}]`)...), 0o644); err != nil {
		t.Fatal(err)
	}
	var out []rec
	if err := Load(p, &out); err != nil {
		t.Fatalf("BOM+JSON: %v", err)
	}
	if len(out) != 1 || out[0].Name != "a" {
		t.Fatalf("BOM+JSON parsed wrong: %v", out)
	}
}

func TestLoad_CorruptNamesThePath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out []rec
	err := Load(p, &out)
	if err == nil {
		t.Fatal("corrupt file must error")
	}
	if !filepath.IsAbs(p) {
		t.Fatal("test invariant: temp path is absolute")
	}
	if got := err.Error(); !strings.Contains(got, "bad.json") {
		t.Fatalf("error should name the file: %v", got)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "roundtrip.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	in := []rec{{Name: "x", N: 7}, {Name: "y", N: 9}}
	if err := Save(p, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	var out []rec
	if err := Load(p, &out); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out) != 2 || out[1].N != 9 {
		t.Fatalf("round trip mismatch: %v", out)
	}
}

func TestLoadFrom_CreatesDirAndReturnsPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh")
	var out []rec
	path, err := LoadFrom(dir, "things.json", &out)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if path != filepath.Join(dir, "things.json") {
		t.Fatalf("wrong path: %s", path)
	}
	// dir must exist so the first Save lands.
	if err := Save(path, []rec{{Name: "z"}}); err != nil {
		t.Fatalf("save into fresh dir: %v", err)
	}
}
