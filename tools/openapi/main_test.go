// SPDX-License-Identifier: MIT

package main

// The generator's own guards.
//
// Two things must hold for the generated artifacts to mean anything:
//
//  1. they are IN SYNC with the sources they are derived from — the catalogue
//     and the registry. A committed document that no longer matches its
//     sources is a lie, so the check is a test, not a habit;
//  2. the browser only ever NAMES paths the console answers. The generated
//     client makes that true for typed calls; this test makes it true for the
//     literals the frontend writes down.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// repoRoot is the repository root, relative to this package's directory.
const repoRoot = "../.."

// pathLiteralRE finds a console path written down as a string literal in a
// TypeScript source file.
var pathLiteralRE = regexp.MustCompile(`["'\x60](/[^"'\x60\s]*)["'\x60]`)

// consolePathPrefixes are the prefixes a console path may start with.
var consolePathPrefixes = []string{"/api", "/hooks", "/oauth", "/events", "/assets", "/favicon"}

// TestTheGeneratedArtifactsAreInSync: the committed document and client must
// be exactly what the catalogue and the registry produce. Regenerate with
// `go run ./tools/openapi` when this fails.
func TestTheGeneratedArtifactsAreInSync(t *testing.T) {
	artifacts, err := generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, art := range artifacts {
		path := filepath.Join(repoRoot, filepath.FromSlash(art.path))
		current, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s is missing: %v", art.path, err)
			continue
		}
		if string(current) != string(art.body) {
			t.Errorf("%s is stale — run `go run ./tools/openapi`", art.path)
		}
	}
}

// TestTheCatalogueAndTheRegistryAgree: every route the console serves must
// name an operation the daemon actually registers, with a wire contract. A
// route pointing at an operation the registry does not know is a route that
// would fail at request time.
func TestTheCatalogueAndTheRegistryAgree(t *testing.T) {
	schemas := registrySchemas()
	for _, route := range opapi.ConsoleRoutes() {
		schema, ok := schemas[route.Op]
		if !ok {
			t.Errorf("%s answers operation %q, which the registry does not know", route.Path, route.Op)
			continue
		}
		if len(schema.Input) == 0 {
			t.Errorf("%s answers operation %q, which declares no input contract", route.Path, route.Op)
		}
		if len(schema.Output) == 0 {
			t.Errorf("%s answers operation %q, which declares no output contract", route.Path, route.Op)
		}
	}
}

// TestTheBrowserOnlyNamesPathsTheConsoleAnswers: no file in the frontend may
// write down a path the console does not answer. A path that is neither a
// catalogue route nor a console-native endpoint is a request that goes
// nowhere — and the compiler cannot catch it when the path is built at
// runtime.
func TestTheBrowserOnlyNamesPathsTheConsoleAnswers(t *testing.T) {
	allowed := map[string]bool{}
	for _, route := range opapi.ConsoleRoutes() {
		allowed[route.Path] = true
	}
	for _, route := range opapi.ConsoleNativeRoutes() {
		allowed[route.Path] = true
	}
	root := filepath.Join(repoRoot, "frontend", "src")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			relative = path
		}
		for _, match := range pathLiteralRE.FindAllStringSubmatch(string(body), -1) {
			literal := match[1]
			if !looksLikeConsolePath(literal) {
				continue
			}
			trimmed := trimPath(literal)
			if trimmed == "" {
				continue
			}
			if !allowed[trimmed] {
				t.Errorf("%s names the console path %q, which is neither a catalogue route nor a console-native endpoint",
					filepath.ToSlash(relative), literal)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the frontend: %v", err)
	}
}

// looksLikeConsolePath reports whether a literal is a console path at all.
func looksLikeConsolePath(path string) bool {
	for _, prefix := range consolePathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// trimPath cuts a literal down to the path the console answers: a query
// string or a template suffix belongs to the CALLER, not to the route.
func trimPath(literal string) string {
	if index := strings.Index(literal, "?"); index >= 0 {
		literal = literal[:index]
	}
	if index := strings.Index(literal, "${"); index >= 0 {
		literal = literal[:index]
	}
	return literal
}

// TestTheDocumentIsValidJSON: the committed document must parse as JSON and
// declare the version it claims.
func TestTheDocumentIsValidJSON(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(openAPIPath)))
	if err != nil {
		t.Fatalf("read the document: %v", err)
	}
	var doc struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the document is not valid JSON: %v", err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Errorf("the document declares OpenAPI %q, want 3.1.0", doc.OpenAPI)
	}
	if len(doc.Paths) != len(opapi.ConsoleRoutes()) {
		t.Errorf("the document describes %d paths, the catalogue has %d", len(doc.Paths), len(opapi.ConsoleRoutes()))
	}
}

// TestTheClientNamesEveryPathItServes: the generated client's unions are the
// console's surface. A path in the catalogue that the client does not name is
// a path the browser cannot call.
func TestTheClientNamesEveryPathItServes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(clientPath)))
	if err != nil {
		t.Fatalf("read the client: %v", err)
	}
	client := string(raw)
	for _, route := range opapi.ConsoleRoutes() {
		if !strings.Contains(client, `"`+route.Path+`"`) {
			t.Errorf("the generated client does not name %s", route.Path)
		}
	}
}
