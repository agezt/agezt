// SPDX-License-Identifier: MIT

package main

// Coverage tests for jsonschemagen. main_test.go covers stripJSONCComments,
// exportedName, refTypeName, and a full end-to-end run over the real contract.
// This file targets the remaining branches: every emitSchema/goTypeFromSchema
// shape, the run() error paths, the dir/typeString/oneLine/writeDocComment
// helpers, mapValueType's boolean form, and main()'s flag-driven success path.
//
// main() exits via os.Exit(1) only on run() error; the coverage writer is
// bypassed by os.Exit, so that single error line stays uncovered. Everything
// else is reachable in-process.

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// emit runs emitSchema for a schema described by the given JSON and returns the
// generated Go source, failing the test on error.
func emit(t *testing.T, name, schemaJSON string) string {
	t.Helper()
	var s schemaNode
	if err := json.Unmarshal([]byte(schemaJSON), &s); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	var buf bytes.Buffer
	if err := emitSchema(&buf, name, s); err != nil {
		t.Fatalf("emitSchema(%s): %v", name, err)
	}
	return buf.String()
}

// TestEmitSchemaScalarTypes covers the string/integer/number/boolean cases of
// emitSchema's type switch, plus the description-driven doc comment.
func TestEmitSchemaScalarTypes(t *testing.T) {
	cases := map[string]struct {
		schema string
		want   string
	}{
		"string":  {`{"type":"string"}`, "type S string"},
		"integer": {`{"type":"integer"}`, "type S int64"},
		"number":  {`{"type":"number"}`, "type S float64"},
		"boolean": {`{"type":"boolean"}`, "type S bool"},
	}
	for kind, c := range cases {
		t.Run(kind, func(t *testing.T) {
			got := emit(t, "S", c.schema)
			if !strings.Contains(got, c.want) {
				t.Errorf("emitSchema %s = %q, want it to contain %q", kind, got, c.want)
			}
		})
	}
}

// TestEmitSchemaObjectVariants covers the three object sub-cases: struct (has
// properties), map (additionalProperties), and opaque (neither).
func TestEmitSchemaObjectVariants(t *testing.T) {
	// object with properties → struct
	got := emit(t, "WithProps", `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}`)
	if !strings.Contains(got, "type WithProps struct {") || !strings.Contains(got, "A string `json:\"a\"`") {
		t.Errorf("object-with-properties did not emit a struct:\n%s", got)
	}

	// object with additionalProperties schema → map[string]T
	got = emit(t, "MapType", `{"type":"object","additionalProperties":{"type":"integer"}}`)
	if !strings.Contains(got, "type MapType map[string]int64") {
		t.Errorf("object-with-additionalProperties did not emit a map:\n%s", got)
	}

	// object with neither → opaque map[string]json.RawMessage
	got = emit(t, "Opaque", `{"type":"object"}`)
	if !strings.Contains(got, "type Opaque map[string]json.RawMessage") {
		t.Errorf("bare object did not emit opaque map:\n%s", got)
	}
}

// TestEmitSchemaArrayVariants covers the array cases: no items (opaque slice)
// and typed items.
func TestEmitSchemaArrayVariants(t *testing.T) {
	got := emit(t, "AnyList", `{"type":"array"}`)
	if !strings.Contains(got, "type AnyList []json.RawMessage") {
		t.Errorf("array-without-items did not emit opaque slice:\n%s", got)
	}

	got = emit(t, "StrList", `{"type":"array","items":{"type":"string"}}`)
	if !strings.Contains(got, "type StrList []string") {
		t.Errorf("array-with-items did not emit typed slice:\n%s", got)
	}
}

// TestEmitSchemaRefAndFallback covers the top-level $ref alias branch and the
// opaque fallback for shapes the generator does not recognise.
func TestEmitSchemaRefAndFallback(t *testing.T) {
	got := emit(t, "Alias", `{"$ref":"#/Event"}`)
	if !strings.Contains(got, "type Alias = Event") {
		t.Errorf("$ref did not emit an alias:\n%s", got)
	}

	// No type, no $ref, no properties → opaque with TODO.
	got = emit(t, "Mystery", `{}`)
	if !strings.Contains(got, "type Mystery json.RawMessage") || !strings.Contains(got, "TODO: jsonschemagen cannot yet map") {
		t.Errorf("unrecognised shape did not emit opaque fallback:\n%s", got)
	}
}

// TestEmitSchemaDefaultDocComment covers the else branch of emitSchema where a
// schema has no description and the generic doc comment is written instead.
func TestEmitSchemaDefaultDocComment(t *testing.T) {
	got := emit(t, "NoDesc", `{"type":"string"}`)
	if !strings.Contains(got, "// NoDesc is defined by the wire contract.") {
		t.Errorf("missing default doc comment:\n%s", got)
	}
}

// TestEmitSchemaDescription covers the writeDocComment path via a schema that
// carries a multi-line description (also exercising oneLine's collapsing).
func TestEmitSchemaDescription(t *testing.T) {
	got := emit(t, "Documented", `{"type":"string","description":"first line\n  second   line"}`)
	if !strings.Contains(got, "// Documented — first line second line") {
		t.Errorf("description was not rendered on one line:\n%s", got)
	}
}

// TestEmitStructOmitempty covers emitStruct's required-vs-optional tag logic:
// required fields have no omitempty, optional ones do.
func TestEmitStructOmitempty(t *testing.T) {
	got := emit(t, "Mixed", `{"type":"object","properties":{"req":{"type":"string"},"opt":{"type":"integer"}},"required":["req"]}`)
	if !strings.Contains(got, "Req string `json:\"req\"`") {
		t.Errorf("required field should not have omitempty:\n%s", got)
	}
	if !strings.Contains(got, "Opt int64 `json:\"opt,omitempty\"`") {
		t.Errorf("optional field should have omitempty:\n%s", got)
	}
}

// TestEmitStructFieldError covers emitStruct's error path: a property whose
// schema is not valid JSON makes goTypeFromSchema fail, which emitStruct wraps.
func TestEmitStructFieldError(t *testing.T) {
	s := schemaNode{
		Type:       json.RawMessage(`"object"`),
		Properties: map[string]json.RawMessage{"bad": json.RawMessage(`{`)}, // invalid JSON
	}
	var buf bytes.Buffer
	if err := emitStruct(&buf, "Broken", s); err == nil {
		t.Fatal("expected emitStruct to fail on an invalid field schema")
	}
}

// TestEmitSchemaArrayItemError covers emitSchema's array-items error branch by
// giving items an invalid schema.
func TestEmitSchemaArrayItemError(t *testing.T) {
	s := schemaNode{
		Type:  json.RawMessage(`"array"`),
		Items: json.RawMessage(`{`), // invalid
	}
	var buf bytes.Buffer
	if err := emitSchema(&buf, "BadArr", s); err == nil {
		t.Fatal("expected emitSchema to fail on invalid array items")
	}
}

// TestEmitSchemaMapValueError covers emitSchema's additionalProperties error
// branch (mapValueType → goTypeFromSchema failing on invalid JSON).
func TestEmitSchemaMapValueError(t *testing.T) {
	s := schemaNode{
		Type:                 json.RawMessage(`"object"`),
		AdditionalProperties: json.RawMessage(`{`), // invalid
	}
	var buf bytes.Buffer
	if err := emitSchema(&buf, "BadMap", s); err == nil {
		t.Fatal("expected emitSchema to fail on invalid additionalProperties")
	}
}

// TestGoTypeFromSchema covers goTypeFromSchema's branches directly, including
// $ref, each scalar, arrays (typed + opaque), object-with-additionalProperties,
// bare object, the unrecognised-shape fallback, and the unmarshal-error path.
func TestGoTypeFromSchema(t *testing.T) {
	cases := map[string]string{
		`{"$ref":"#/Event"}`:                         "Event",
		`{"type":"string"}`:                          "string",
		`{"type":"integer"}`:                         "int64",
		`{"type":"number"}`:                          "float64",
		`{"type":"boolean"}`:                         "bool",
		`{"type":"array","items":{"type":"string"}}`: "[]string",
		`{"type":"array"}`:                           "[]json.RawMessage",
		`{"type":"object","additionalProperties":{"type":"boolean"}}`: "map[string]bool",
		`{"type":"object"}`: "map[string]json.RawMessage",
		`{}`:                "json.RawMessage",
	}
	for in, want := range cases {
		got, err := goTypeFromSchema(json.RawMessage(in))
		if err != nil {
			t.Errorf("goTypeFromSchema(%s) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("goTypeFromSchema(%s) = %q, want %q", in, got, want)
		}
	}

	// Invalid JSON → unmarshal error.
	if _, err := goTypeFromSchema(json.RawMessage(`{`)); err == nil {
		t.Error("expected unmarshal error for invalid schema JSON")
	}

	// Nested array-items error path.
	if _, err := goTypeFromSchema(json.RawMessage(`{"type":"array","items":{`)); err == nil {
		t.Error("expected error propagation from invalid array items")
	}

	// Object additionalProperties error path.
	if _, err := goTypeFromSchema(json.RawMessage(`{"type":"object","additionalProperties":{`)); err == nil {
		t.Error("expected error propagation from invalid additionalProperties")
	}
}

// TestMapValueType covers both branches of mapValueType: a bare `true`
// (additionalProperties: true → json.RawMessage) and a schema node.
func TestMapValueType(t *testing.T) {
	got, err := mapValueType(json.RawMessage(`true`))
	if err != nil || got != "json.RawMessage" {
		t.Errorf("mapValueType(true) = %q, %v; want json.RawMessage, nil", got, err)
	}

	got, err = mapValueType(json.RawMessage(`{"type":"string"}`))
	if err != nil || got != "string" {
		t.Errorf("mapValueType(schema) = %q, %v; want string, nil", got, err)
	}
}

// TestTypeString covers schemaNode.typeString: empty type, a single string
// type, and the multi-type array fallback that returns "".
func TestTypeString(t *testing.T) {
	if got := (schemaNode{}).typeString(); got != "" {
		t.Errorf("empty type = %q, want empty", got)
	}
	if got := (schemaNode{Type: json.RawMessage(`"string"`)}).typeString(); got != "string" {
		t.Errorf("single type = %q, want string", got)
	}
	// A JSON array of types is valid JSON Schema but unsupported → "".
	if got := (schemaNode{Type: json.RawMessage(`["string","null"]`)}).typeString(); got != "" {
		t.Errorf("multi type = %q, want empty (fallthrough)", got)
	}
}

// TestWriteDocCommentEmpty covers writeDocComment's early return when the
// (trimmed) description is empty.
func TestWriteDocCommentEmpty(t *testing.T) {
	var buf bytes.Buffer
	writeDocComment(&buf, "Name", "   \n  ")
	if buf.Len() != 0 {
		t.Errorf("expected no output for blank description, got %q", buf.String())
	}
}

// TestOneLine covers oneLine directly: carriage returns and newlines become
// spaces and runs of spaces collapse to one.
func TestOneLine(t *testing.T) {
	got := oneLine("a\r\nb   c\n\nd")
	if got != "a b c d" {
		t.Errorf("oneLine = %q, want %q", got, "a b c d")
	}
}

// TestDir covers dir's three cases: a forward-slash path, a backslash path, and
// a bare name with no separator (which returns ".").
func TestDir(t *testing.T) {
	if got := dir("a/b/c.go"); got != "a/b" {
		t.Errorf("dir(forward) = %q, want a/b", got)
	}
	if got := dir(`a\b\c.go`); got != `a\b` {
		t.Errorf("dir(back) = %q, want a\\b", got)
	}
	if got := dir("plain.go"); got != "." {
		t.Errorf("dir(no separator) = %q, want .", got)
	}
}

// TestExportedNameEmptyPart covers the empty-part continue branch of
// exportedName (a name with a leading/trailing/double underscore).
func TestExportedNameEmptyPart(t *testing.T) {
	if got := exportedName("_leading"); got != "Leading" {
		t.Errorf("exportedName(_leading) = %q, want Leading", got)
	}
	if got := exportedName("a__b"); got != "AB" {
		t.Errorf("exportedName(a__b) = %q, want AB", got)
	}
}

// TestRunReadError covers run's os.ReadFile error branch.
func TestRunReadError(t *testing.T) {
	err := run(filepath.Join(t.TempDir(), "missing.jsonc"), filepath.Join(t.TempDir(), "out.go"), "gen")
	if err == nil {
		t.Fatal("expected run to fail reading a missing input file")
	}
}

// TestRunParseError covers run's json.Unmarshal error branch with malformed
// contract content.
func TestRunParseError(t *testing.T) {
	in := writeTemp(t, "bad.jsonc", `"$schemas": not-json`)
	err := run(in, filepath.Join(t.TempDir(), "out.go"), "gen")
	if err == nil || !strings.Contains(err.Error(), "parse contract") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

// TestRunEmptySchemas covers run's "no $schemas block" error branch.
func TestRunEmptySchemas(t *testing.T) {
	in := writeTemp(t, "empty.jsonc", `"$schemas": {}`)
	err := run(in, filepath.Join(t.TempDir(), "out.go"), "gen")
	if err == nil || !strings.Contains(err.Error(), "no $schemas block") {
		t.Fatalf("expected empty-schemas error, got %v", err)
	}
}

// TestRunSchemaUnmarshalError covers run's per-schema json.Unmarshal error
// branch, where a schema value is not a JSON object.
func TestRunSchemaUnmarshalError(t *testing.T) {
	in := writeTemp(t, "badschema.jsonc", `"$schemas": {"Bad": 123}`)
	err := run(in, filepath.Join(t.TempDir(), "out.go"), "gen")
	if err == nil || !strings.Contains(err.Error(), "schema Bad") {
		t.Fatalf("expected schema-unmarshal error, got %v", err)
	}
}

// TestRunSuccess covers run's full happy path — including writeHeader, emit,
// gofmt, mkdir, and file write — by driving it over a small valid contract into
// a nested (not-yet-existing) output directory so os.MkdirAll runs.
func TestRunSuccess(t *testing.T) {
	in := writeTemp(t, "good.jsonc", `"$schemas": {
		"Widget": {"type":"object","properties":{"id":{"type":"string"}},"required":["id"]},
		"Kind": {"type":"string"}
	}`)
	out := filepath.Join(t.TempDir(), "nested", "types.gen.go")
	if err := run(in, out, "gen"); err != nil {
		t.Fatalf("run success path failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "package gen") || !strings.Contains(s, "type Widget struct") {
		t.Errorf("generated output missing expected content:\n%s", s)
	}
}

// TestRunEmitError covers run's emitSchema error branch: a schema that parses
// at the top level but has an array whose items are an invalid schema, so
// emitSchema (called from run) returns an error that run wraps as "emit ...".
func TestRunEmitError(t *testing.T) {
	// items is a string, not a schema object, so goTypeFromSchema (via
	// emitSchema's array branch) fails to unmarshal it into a schemaNode.
	in := writeTemp(t, "emitfail.jsonc", `"$schemas": {"Bad": {"type":"array","items":"not-a-schema-object"}}`)
	err := run(in, filepath.Join(t.TempDir(), "out.go"), "gen")
	if err == nil || !strings.Contains(err.Error(), "emit Bad") {
		t.Fatalf("expected emit error, got %v", err)
	}
}

// TestRunGofmtError covers run's format.Source failure branch. A schema name
// containing a space produces syntactically invalid Go (`type has name string`),
// which gofmt rejects; run then writes the unformatted debug file and returns a
// "gofmt" error.
func TestRunGofmtError(t *testing.T) {
	in := writeTemp(t, "gofmtfail.jsonc", `"$schemas": {"has name": {"type":"string"}}`)
	out := filepath.Join(t.TempDir(), "out.go")
	err := run(in, out, "gen")
	if err == nil || !strings.Contains(err.Error(), "gofmt") {
		t.Fatalf("expected gofmt error, got %v", err)
	}
	// The unformatted debug file should have been written alongside.
	if _, statErr := os.Stat(out + ".unformatted"); statErr != nil {
		t.Errorf("expected unformatted debug file at %s.unformatted: %v", out, statErr)
	}
}

// TestGoTypeFromSchemaNestedErrors covers goTypeFromSchema's two propagated
// error branches: invalid nested array items and invalid nested
// additionalProperties, where the OUTER JSON is valid but the inner schema is
// not a JSON object.
func TestGoTypeFromSchemaNestedErrors(t *testing.T) {
	// Outer array parses; inner items is a bare string, not a schema object,
	// so the recursive goTypeFromSchema call fails.
	if _, err := goTypeFromSchema(json.RawMessage(`{"type":"array","items":"nope"}`)); err == nil {
		t.Error("expected error from invalid nested array items")
	}
	// Outer object parses; additionalProperties is a number (neither bool nor
	// object), so mapValueType → goTypeFromSchema fails.
	if _, err := goTypeFromSchema(json.RawMessage(`{"type":"object","additionalProperties":42}`)); err == nil {
		t.Error("expected error from invalid nested additionalProperties")
	}
}

// TestRunMkdirError covers run's os.MkdirAll error branch. The output path is
// placed *under* an existing regular file, so dir(outPath) names a path whose
// parent component is a file — os.MkdirAll cannot create a directory there.
func TestRunMkdirError(t *testing.T) {
	in := writeTemp(t, "mkdir.jsonc", `"$schemas": {"Ping": {"type":"string"}}`)

	// Create a regular file, then ask run to write "under" it: dir(outPath)
	// becomes the file path, and MkdirAll("<file>/sub") fails.
	blocker := writeTemp(t, "blocker", "not a directory")
	out := filepath.Join(blocker, "sub", "types.gen.go")

	err := run(in, out, "gen")
	if err == nil || !strings.Contains(err.Error(), "mkdir") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

// TestRunWriteError covers run's os.WriteFile error branch. The output path is
// an existing directory: dir(outPath) resolves to its already-present parent so
// MkdirAll succeeds, but os.WriteFile cannot write file bytes over a directory.
func TestRunWriteError(t *testing.T) {
	in := writeTemp(t, "write.jsonc", `"$schemas": {"Ping": {"type":"string"}}`)

	// Make outPath itself a directory.
	outDir := filepath.Join(t.TempDir(), "types.gen.go")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	err := run(in, outDir, "gen")
	if err == nil || !strings.Contains(err.Error(), "write") {
		t.Fatalf("expected write error, got %v", err)
	}
}

// TestMainSuccess drives main() in-process with -in/-out/-pkg flags, exercising
// flag parsing and the run() success branch (no os.Exit). Only the os.Exit error
// branch remains uncovered.
func TestMainSuccess(t *testing.T) {
	in := writeTemp(t, "main.jsonc", `"$schemas": {"Ping": {"type":"string"}}`)
	out := filepath.Join(t.TempDir(), "out.go")

	origArgs := os.Args
	origFlags := flag.CommandLine
	defer func() {
		os.Args = origArgs
		flag.CommandLine = origFlags
	}()
	flag.CommandLine = flag.NewFlagSet("jsonschemagen", flag.ExitOnError)
	os.Args = []string{"jsonschemagen", "-in", in, "-out", out, "-pkg", "gen"}

	main()

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("main did not produce output: %v", err)
	}
}

// writeTemp writes content to a uniquely-named file in a fresh temp dir and
// returns its path.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp %s: %v", name, err)
	}
	return path
}
