// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app/board"
	"github.com/agezt/agezt/kernel/contract/restapiapi"
)

// repoRoot is the repository root, one level above this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

// TestTheGeneratedCatalogsAreInSync is the drift gate: a change in the
// contract that nobody regenerated must fail here, not in a customer's SDK.
func TestTheGeneratedCatalogsAreInSync(t *testing.T) {
	files, err := generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	root := repoRoot(t)
	for _, f := range files {
		have, err := os.ReadFile(filepath.Join(root, f.Path))
		if err != nil {
			t.Fatalf("read %s: %v", f.Path, err)
		}
		if !bytes.Equal(have, f.Body) {
			t.Errorf("%s is stale — run `go run ./tools/sdkgen`", f.Path)
		}
	}
}

// sdkLang is one SDK the ratchets look at.
type sdkLang struct {
	// Name is the language's name.
	Name string
	// Glob is the SDK's source glob.
	Glob string
	// Generated is the generated file inside the glob (it owns the paths).
	Generated string
	// Clients are the files that must reach every contract path.
	Clients []string
	// Ref renders the reference to a path constant in this language.
	Ref func(name string) string
}

// sdks are the SDKs wired to the generated catalog. The Go SDK is not among
// them: it speaks the control plane, so it has no REST paths — its generated
// file carries the board's wire names instead.
func sdks() []sdkLang {
	return []sdkLang{
		{
			Name:      "python",
			Glob:      "sdk/python/agezt/*.py",
			Generated: "sdk/python/agezt/_generated.py",
			Clients:   []string{"sdk/python/agezt/client.py"},
			Ref:       func(name string) string { return "Paths." + upperSnake(name) },
		},
		{
			Name:      "typescript",
			Glob:      "sdk/typescript/src/*.ts",
			Generated: "sdk/typescript/src/generated.ts",
			Clients:   []string{"sdk/typescript/src/client.ts"},
			Ref:       func(name string) string { return "PATHS." + camel(name) },
		},
	}
}

// TestTheSDKsNameNoPathOfTheirOwn is the ratchet that keeps the SDK surface
// single-sourced: outside the generated catalog, no SDK source may spell a
// REST path. A hand-written path is a path the contract cannot see.
func TestTheSDKsNameNoPathOfTheirOwn(t *testing.T) {
	root := repoRoot(t)
	cat := buildCatalog()
	for _, lang := range sdks() {
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(lang.Glob)))
		if err != nil {
			t.Fatalf("glob %s: %v", lang.Glob, err)
		}
		if len(files) == 0 {
			t.Fatalf("no %s SDK source matched %q", lang.Name, lang.Glob)
		}
		generated := filepath.ToSlash(filepath.Join(root, filepath.FromSlash(lang.Generated)))
		for _, f := range files {
			if filepath.ToSlash(f) == generated {
				continue
			}
			body, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			for _, p := range cat.Paths {
				if !strings.Contains(string(body), p.Path) {
					continue
				}
				t.Errorf("%s spells the REST path %q of its own", filepath.ToSlash(f), p.Path)
			}
		}
	}
}

// TestEveryContractPathIsReachedByEverySDK is the coverage gate that replaces
// the retired sdkparity report: an endpoint the contract puts in the SDK
// contract must be reachable in every SDK, and the only way to reach it is
// through the generated path constant.
func TestEveryContractPathIsReachedByEverySDK(t *testing.T) {
	root := repoRoot(t)
	cat := buildCatalog()
	for _, lang := range sdks() {
		body := ""
		for _, c := range lang.Clients {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c)))
			if err != nil {
				t.Fatalf("read %s: %v", c, err)
			}
			body += string(raw)
		}
		for _, p := range cat.Paths {
			if !strings.Contains(body, lang.Ref(p.Name)) {
				t.Errorf("the %s SDK never calls %q (%s is missing)", lang.Name, p.Path, lang.Ref(p.Name))
			}
		}
	}
}

// TestTheRestContractAgreesWithTheWireProjection keeps the contract's message
// row honest: the app layer projects the board's message into the row the
// clients read, and the contract's row must name the same fields.
func TestTheRestContractAgreesWithTheWireProjection(t *testing.T) {
	assertRowAgrees(t, restapiapi.RowMail, reflect.TypeOf(board.Message{}))
	assertRowAgrees(t, restapiapi.RowMailDraft, reflect.TypeOf(board.SendInput{}))
}

// assertRowAgrees checks that every field of a contract row is a field of the
// wire projection, under the same name.
func assertRowAgrees(t *testing.T, rowName string, projection reflect.Type) {
	t.Helper()
	var row restapiapi.Row
	found := false
	for _, r := range restapiapi.Rows() {
		if r.Name == rowName {
			row, found = r, true
			break
		}
	}
	if !found {
		t.Fatalf("the contract has no %q row", rowName)
	}
	wire := map[string]bool{}
	for _, f := range wireFields(projection) {
		wire[f.Wire] = true
	}
	for _, f := range row.Fields {
		if !wire[f.Name] {
			t.Errorf("the contract's %q row names %q, which %s does not carry", rowName, f.Name, projection.Name())
		}
	}
}
