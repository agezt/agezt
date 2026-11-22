// SPDX-License-Identifier: MIT

// Command openapi generates the daemon's API artifacts from its own sources.
//
// Two artifacts come out of this command, and both are DERIVED:
//
//   - docs/openapi.json — the OpenAPI 3.1 document of the console's HTTP
//     surface, joined from the console's route catalogue (which path answers
//     which operation, with which verb and arguments) and the daemon's
//     operation registry (what each operation accepts and returns).
//   - frontend/src/app/api/client.ts — the browser's client, whose path
//     unions and verbs are the same two sources.
//   - frontend/src/app/api/types.generated.ts — the browser's payload types,
//     rendered from the registry's JSON Schema, so the browser stops writing
//     by hand what the registry already describes.
//
// Neither file is written by hand. A hand-written copy of either would be a
// second source of truth that drifts; the whole point of this slice is that
// the browser and the daemon cannot disagree about a route without the
// compiler or this command saying so.
//
// Usage:
//
//	go run ./tools/openapi            # regenerate the artifacts
//	go run ./tools/openapi -check     # fail if the committed files are stale
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/controlplane"
)

const (
	// openAPIPath is where the API document lives.
	openAPIPath = "docs/openapi.json"
	// clientPath is where the generated browser client lives.
	clientPath = "frontend/src/app/api/client.ts"
	// typesPath is where the generated browser payload types live.
	typesPath = "frontend/src/app/api/types.generated.ts"
)

func main() {
	root := flag.String("root", ".", "the repository root the artifacts live in")
	check := flag.Bool("check", false, "verify the committed artifacts match the generated ones")
	flag.Parse()
	if err := run(*root, *check); err != nil {
		fmt.Fprintln(os.Stderr, "openapi:", err)
		os.Exit(1)
	}
}

// artifact is one generated file.
type artifact struct {
	path string
	body []byte
}

// run generates the artifacts and either writes or checks them.
func run(root string, check bool) error {
	artifacts, err := generate()
	if err != nil {
		return err
	}
	if check {
		return verify(root, artifacts)
	}
	for _, art := range artifacts {
		if err := write(root, art); err != nil {
			return err
		}
		fmt.Println("wrote", art.path)
	}
	return nil
}

// registrySchemas returns the daemon's operation wire contracts, keyed by
// operation name.
func registrySchemas() map[string]opapi.OperationSchema {
	return controlplane.AppOperationSchemas()
}

// generate builds the artifacts from the catalogue and the registry.
func generate() ([]artifact, error) {
	schemas := registrySchemas()
	source := func(op string) (opapi.OperationSchema, bool) {
		schema, ok := schemas[op]
		return schema, ok
	}
	doc, err := opapi.BuildOpenAPI(source, opapi.OpenAPIInfo{
		Title:   "AGEZT Console API",
		Version: brand.Version,
		Description: "The console's HTTP surface, derived from the daemon's operation registry " +
			"and the console's route catalogue. The console's own endpoints (login, files, " +
			"voice, rollback, the event feed) are not operation bindings and are not described here.",
	})
	if err != nil {
		return nil, fmt.Errorf("build the API document: %w", err)
	}
	// The document describes the catalogue, operation for operation: a route
	// that lost its operation, or an operation that appeared twice, would
	// leave the document describing a surface the console does not serve.
	if documented, catalogued := len(opapi.OpenAPIOperationIDs(doc)), len(opapi.ConsoleRoutes()); documented != catalogued {
		return nil, fmt.Errorf("the document describes %d operations, the catalogue has %d", documented, catalogued)
	}
	raw, err := renderOpenAPI(doc)
	if err != nil {
		return nil, err
	}
	client := renderClient(opapi.ConsoleRoutes(), opapi.ConsoleNativeRoutes())
	types, err := renderTypes(opapi.ConsoleRoutes(), doc)
	if err != nil {
		return nil, err
	}
	return []artifact{
		{path: openAPIPath, body: raw},
		{path: clientPath, body: []byte(client)},
		{path: typesPath, body: []byte(types)},
	}, nil
}

// write puts one artifact on disk, creating its directory if needed.
func write(root string, art artifact) error {
	full := filepath.Join(root, filepath.FromSlash(art.path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(art.path), err)
	}
	if err := os.WriteFile(full, art.body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", art.path, err)
	}
	return nil
}

// verify fails when a committed artifact differs from the generated one. This
// is the gate: a stale document or a stale client is a drift the repository
// must not carry.
func verify(root string, artifacts []artifact) error {
	var stale []string
	for _, art := range artifacts {
		full := filepath.Join(root, filepath.FromSlash(art.path))
		current, err := os.ReadFile(full)
		if err != nil {
			stale = append(stale, fmt.Sprintf("%s (%v)", art.path, err))
			continue
		}
		if string(current) != string(art.body) {
			stale = append(stale, art.path)
		}
	}
	if len(stale) > 0 {
		return fmt.Errorf("the generated artifacts are stale: %v — run `go run ./tools/openapi`", stale)
	}
	for _, art := range artifacts {
		fmt.Println("in sync", art.path)
	}
	return nil
}
