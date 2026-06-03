// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
)

func TestPluginNew_ScaffoldsBuildableProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myplugin")
	var out, errb bytes.Buffer
	code := cmdPluginNew([]string{"myplugin", "--dir", dir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errb.String())
	}

	// All four scaffold files exist.
	for _, f := range []string{"main.go", "go.mod", "README.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing scaffold file %s: %v", f, err)
		}
	}

	// main.go is valid, gofmt-clean Go that imports the SDK and
	// registers the derived tool.
	src, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", src, parser.AllErrors); err != nil {
		t.Fatalf("generated main.go does not parse: %v", err)
	}
	formatted, err := format.Source(src)
	if err != nil {
		t.Fatalf("format.Source: %v", err)
	}
	if !bytes.Equal(formatted, src) {
		t.Error("generated main.go is not gofmt-clean")
	}
	s := string(src)
	if !strings.Contains(s, `"github.com/agezt/agezt/plugins/sdk"`) {
		t.Error("main.go does not import the SDK")
	}
	if !strings.Contains(s, `Name:        "myplugin"`) {
		t.Error("main.go does not register the myplugin tool")
	}

	// go.mod declares the module and requires the agezt SDK at the
	// current product version.
	gomod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	gs := string(gomod)
	if !strings.Contains(gs, "module agezt-plugin-myplugin") {
		t.Errorf("go.mod module line wrong:\n%s", gs)
	}
	if !strings.Contains(gs, "require github.com/agezt/agezt v"+brand.Version) {
		t.Errorf("go.mod require line missing or wrong version:\n%s", gs)
	}

	// The success message tells the operator how to wire it.
	if !strings.Contains(out.String(), "AGEZT_PLUGINS") {
		t.Error("success output should mention AGEZT_PLUGINS wiring")
	}
}

func TestPluginNew_DefaultDirIsName(t *testing.T) {
	base := t.TempDir()
	// cmdPluginNew uses a relative default dir (the name); run from base.
	t.Chdir(base)
	var out, errb bytes.Buffer
	if code := cmdPluginNew([]string{"toolio"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(base, "toolio", "main.go")); err != nil {
		t.Errorf("expected ./toolio/main.go: %v", err)
	}
}

func TestPluginNew_ModuleOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p")
	var out, errb bytes.Buffer
	code := cmdPluginNew([]string{"p", "--dir", dir, "--module", "example.com/me/coolplugin"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, errb.String())
	}
	gomod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(gomod), "module example.com/me/coolplugin") {
		t.Errorf("module override not honoured:\n%s", gomod)
	}
}

func TestPluginNew_RefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir() // already exists and...
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := cmdPluginNew([]string{"thing", "--dir", dir}, &out, &errb)
	if code == 0 {
		t.Fatal("expected non-zero exit for non-empty dir")
	}
	if !strings.Contains(errb.String(), "not empty") {
		t.Errorf("stderr = %q, want 'not empty'", errb.String())
	}
	// The pre-existing file is untouched.
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Error("scaffolder disturbed an existing file")
	}
}

func TestPluginNew_RequiresName(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdPluginNew(nil, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, want 2 for missing name", code)
	}
	if !strings.Contains(errb.String(), "name is required") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestPluginNew_RejectsUnknownFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdPluginNew([]string{"x", "--bogus"}, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, want 2 for unknown flag", code)
	}
}

func TestPluginNew_NameWithNoUsableChars(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	var out, errb bytes.Buffer
	if code := cmdPluginNew([]string{"!!!", "--dir", dir}, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, want 2 for unusable name", code)
	}
}

func TestSanitizeToolName(t *testing.T) {
	cases := map[string]string{
		"weather":        "weather",
		"My Cool Plugin": "my-cool-plugin",
		"foo.bar/baz":    "foo-bar-baz",
		"  spaced  ":     "spaced",
		"under_score":    "under_score",
		"a---b":          "a-b",
		"!!!":            "",
		"Café9":          "caf9",
	}
	for in, want := range cases {
		if got := sanitizeToolName(in); got != want {
			t.Errorf("sanitizeToolName(%q) = %q, want %q", in, got, want)
		}
	}
}
