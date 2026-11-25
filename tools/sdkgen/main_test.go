// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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
	// FreeForm is the shape of a hand-built request body in this language: a
	// body the client spells itself, free of the catalog's types. Empty when
	// the language has no such shape to forbid.
	FreeForm string
	// WireKey is how this language spells a wire name as a body key, with %s
	// the wire name. Empty when the plain quoted key covers it.
	WireKey string
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
			FreeForm:  `Dict\[str, Any\] = \{[^}]`,
		},
		{
			Name:      "typescript",
			Glob:      "sdk/typescript/src/*.ts",
			Generated: "sdk/typescript/src/generated.ts",
			Clients:   []string{"sdk/typescript/src/client.ts"},
			Ref:       func(name string) string { return "PATHS." + camel(name) },
			FreeForm:  `Record<string, unknown> = \{[^}]`,
		},
		{
			Name:      "rust",
			Glob:      "sdk/rust/src/*.rs",
			Generated: "sdk/rust/src/generated.rs",
			Clients:   []string{"sdk/rust/src/client.rs"},
			Ref:       func(name string) string { return "paths::" + upperSnake(name) },
			// The Rust SDK has no free-form body shape to forbid: it has always
			// built its bodies from the generated rows, and its `Value::Object`
			// calls parse the daemon's ANSWERS.
			FreeForm: "",
			WireKey:  `insert\(\"%s\"`,
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
			if !mentions(body, lang.Ref(p.Name)) {
				t.Errorf("the %s SDK never calls %q (%s is missing)", lang.Name, p.Path, lang.Ref(p.Name))
			}
		}
	}
}

// TestTheSDKsBuildTheirRequestBodiesFromTheCatalog is the ratchet that keeps
// the SDK request side single-sourced: an SDK client may not spell a request
// body of its own.
//
// The wire names of a request row live in the contract and in the generated
// catalog — nowhere else. Three shapes would smuggle a second vocabulary back
// in:
//
//  1. a hand-built body: the free-form literal the client spells itself;
//  2. a quoted wire-name KEY: the client naming a row's field on the wire
//     (the contract's query parameters are a different vocabulary and are not
//     caught here);
//  3. the wire name of a field the language RENAMES: the difference between
//     the two spellings lives in the catalog, so the client may not use the
//     wire one (`reply_to` in TypeScript, `from` in Python).
//
// A client that builds its body from the generated row says the field in the
// language's own name, and the catalog does the rest.
func TestTheSDKsBuildTheirRequestBodiesFromTheCatalog(t *testing.T) {
	root := repoRoot(t)
	for _, lang := range sdks() {
		for _, c := range lang.Clients {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c)))
			if err != nil {
				t.Fatalf("read %s: %v", c, err)
			}
			body := stripProse(string(raw), lang.Name)
			if lang.FreeForm != "" && regexp.MustCompile(lang.FreeForm).MatchString(body) {
				t.Errorf("%s builds a request body of its own (%q) — the body's shape belongs to the generated row",
					c, lang.FreeForm)
			}
			for _, row := range restapiapi.RequestRows() {
				for _, f := range row.Fields {
					if strings.Contains(body, "\""+f.Name+"\":") || strings.Contains(body, "'"+f.Name+"':") {
						t.Errorf("%s spells the request field %q on the wire — the wire names live in the generated catalog",
							c, f.Name)
					}
					if lang.WireKey != "" {
						key := regexp.MustCompile(fmt.Sprintf(lang.WireKey, regexp.QuoteMeta(f.Name)))
						if key.MatchString(body) {
							t.Errorf("%s spells the request field %q on the wire — the wire names live in the generated catalog",
								c, f.Name)
						}
					}
					local := f.LocalName(lang.Name)
					if local == f.Name {
						continue
					}
					if usesWireName(body, f.Name) {
						t.Errorf("%s spells the wire name %q of a field it calls %q — the rename lives in the generated catalog",
							c, f.Name, local)
					}
				}
			}
		}
	}
}

// TestEveryRequestRowIsReachedByEverySDK is the coverage gate for the request
// side: a row the contract puts in the SDK contract must be built by every
// SDK, and the only way to build it is through the generated row.
func TestEveryRequestRowIsReachedByEverySDK(t *testing.T) {
	root := repoRoot(t)
	for _, lang := range sdks() {
		body := ""
		for _, c := range lang.Clients {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c)))
			if err != nil {
				t.Fatalf("read %s: %v", c, err)
			}
			body += string(raw)
		}
		for _, row := range restapiapi.RequestRows() {
			name := pascal(row.Name)
			if !mentions(body, name) {
				t.Errorf("the %s SDK never builds the %q request row (%s is missing)",
					lang.Name, row.Name, name)
			}
		}
	}
}

// usesWireName reports whether a source uses a wire name as a quoted string or
// as a property: the two ways a client can spell a field on the wire.
func usesWireName(body, name string) bool {
	quoted := regexp.MustCompile(`["']` + regexp.QuoteMeta(name) + `["']`)
	if quoted.MatchString(body) {
		return true
	}
	// A property access: `.name` and not `.name_something`.
	prop := regexp.MustCompile(`\.` + regexp.QuoteMeta(name) + `([^A-Za-z0-9_]|$)`)
	return prop.MatchString(body)
}

// stripProse removes what is not code: the comments and the doc strings, and
// (in Rust) the test module — a test may name the wire on purpose, and the
// SDK's own doc comments show examples that do.
func stripProse(src, lang string) string {
	switch lang {
	case "python":
		src = regexp.MustCompile(`(?s)"""..*?"""`).ReplaceAllString(src, "")
		src = regexp.MustCompile(`(?s)'''..*?'''`).ReplaceAllString(src, "")
		src = regexp.MustCompile(`#[^\n]*`).ReplaceAllString(src, "")
	case "rust":
		if i := strings.Index(src, "#[cfg(test)]"); i >= 0 {
			src = src[:i]
		}
		src = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
		src = regexp.MustCompile(`//[^\n]*`).ReplaceAllString(src, "")
	default:
		src = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
		src = regexp.MustCompile(`//[^\n]*`).ReplaceAllString(src, "")
	}
	return src
}

// mentions reports whether the body mentions the reference as a WHOLE
// identifier. Without it, a client that calls only `Paths.RUN_ARC` would look
// like a client that also calls `Paths.RUN` — the prefix is not the path.
func mentions(body, ref string) bool {
	for i := 0; i < len(body); {
		j := strings.Index(body[i:], ref)
		if j < 0 {
			return false
		}
		j += i
		i = j + len(ref)
		if i >= len(body) {
			return true
		}
		c := body[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' {
			continue
		}
		return true
	}
	return false
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
