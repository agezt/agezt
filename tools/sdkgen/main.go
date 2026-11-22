// SPDX-License-Identifier: MIT

// Command sdkgen generates the SDK catalogs from the daemon's REST contract.
//
// The contract lives in kernel/contract/restapiapi: the routes the daemon
// registers, the endpoints an SDK can call and the wire rows they exchange.
// This command renders that one table into each SDK's own language:
//
//	sdk/python/agezt/_generated.py   paths + wire rows (dataclasses / TypedDicts)
//	sdk/typescript/src/generated.ts   paths + wire rows (interfaces)
//	sdk/rust/src/generated.rs        paths + wire rows (flat structs)
//	sdk/generated.go                  the board row's wire field names
//
// The hand-written clients use the generated names, so a path or a wire field
// exists in exactly one place. `-check` fails when a generated file is stale —
// the same drift gate W5.1 established for the OpenAPI document and the
// browser client.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/app/board"
	"github.com/agezt/agezt/kernel/contract/restapiapi"
)

// generatedFile is one artifact this command owns.
type generatedFile struct {
	// Path is the file's path relative to the repository root.
	Path string
	// Body is the file's content.
	Body []byte
}

// pathSpec is one unique path of the SDK contract.
type pathSpec struct {
	// Name is the endpoint id that named the path ("mailbox_ack").
	Name string
	// Path is the path template ("/api/v1/mailbox/messages/{id}/ack").
	Path string
	// Doc is one line of prose about the endpoint that named the path.
	Doc string
}

// catalog is the generated view of the contract.
type catalog struct {
	// Paths are the SDK contract's unique paths, in contract order.
	Paths []pathSpec
	// Rows are the wire rows, in contract order.
	Rows []restapiapi.Row
}

// pythonClassRows are the rows the Python SDK models as dataclasses (the
// ones its public API already exposes as classes). Every other row becomes a
// TypedDict: the Python SDK returns plain dicts for those, and a TypedDict
// documents the shape without changing what the caller receives.
var pythonClassRows = map[string]bool{
	restapiapi.RowRunResult: true,
	restapiapi.RowMail:      true,
	restapiapi.RowMailDraft: true,
}

func main() {
	root := flag.String("root", ".", "repository root")
	check := flag.Bool("check", false, "verify the generated SDK catalogs are in sync")
	flag.Parse()

	files, err := generate()
	if err != nil {
		fatal(err)
	}
	if *check {
		stale := 0
		for _, f := range files {
			want := f.Path
			have, err := os.ReadFile(filepath.Join(*root, want))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: missing (run `go run ./tools/sdkgen`)\n", want)
				stale++
				continue
			}
			if !bytes.Equal(have, f.Body) {
				fmt.Fprintf(os.Stderr, "%s: stale (run `go run ./tools/sdkgen`)\n", want)
				stale++
			}
		}
		if stale > 0 {
			os.Exit(1)
		}
		return
	}
	for _, f := range files {
		p := filepath.Join(*root, f.Path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(p, f.Body, 0o644); err != nil {
			fatal(err)
		}
		fmt.Println("wrote " + f.Path)
	}
}

// generate renders every generated file.
func generate() ([]generatedFile, error) {
	cat := buildCatalog()
	goBody, err := renderGo(cat)
	if err != nil {
		return nil, err
	}
	return []generatedFile{
		{Path: filepath.FromSlash("sdk/python/agezt/_generated.py"), Body: renderPython(cat)},
		{Path: filepath.FromSlash("sdk/typescript/src/generated.ts"), Body: renderTypeScript(cat)},
		{Path: filepath.FromSlash("sdk/rust/src/generated.rs"), Body: renderRust(cat)},
		{Path: filepath.FromSlash("sdk/generated.go"), Body: goBody},
	}, nil
}

// buildCatalog turns the contract into the generated view: the SDK contract's
// unique paths (named after the endpoint that introduced them) and the wire
// rows.
func buildCatalog() catalog {
	eps := restapiapi.Endpoints()
	sort.Slice(eps, func(i, j int) bool { return eps[i].ID < eps[j].ID })
	seen := map[string]bool{}
	var paths []pathSpec
	for _, e := range eps {
		if !e.SDK || seen[e.Path] {
			continue
		}
		seen[e.Path] = true
		paths = append(paths, pathSpec{Name: e.ID, Path: e.Path, Doc: e.Doc})
	}
	return catalog{Paths: paths, Rows: restapiapi.Rows()}
}

// --- naming helpers -------------------------------------------------------

// upperSnake turns an endpoint id into a constant name ("run_arc" ->
// "RUN_ARC").
func upperSnake(id string) string { return strings.ToUpper(id) }

// camel turns an endpoint id into a camelCase name ("run_arc" -> "runArc").
func camel(id string) string {
	parts := strings.Split(id, "_")
	out := parts[0]
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		out += strings.ToUpper(p[:1]) + p[1:]
	}
	return out
}

// pascal turns a row name into a type name ("run_result" -> "RunResult").
func pascal(name string) string {
	parts := strings.Split(name, "_")
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		out += strings.ToUpper(p[:1]) + p[1:]
	}
	return out
}

// --- Python ----------------------------------------------------------------

// renderPython renders the Python SDK's catalog: the paths and the wire rows.
func renderPython(cat catalog) []byte {
	var b bytes.Buffer
	pythonHeader(&b)
	b.WriteString("\n\nclass Paths:\n")
	b.WriteString(`    """The daemon's REST paths (generated from the contract)."""` + "\n\n")
	for _, p := range cat.Paths {
		fmt.Fprintf(&b, "    %s = %q\n", upperSnake(p.Name), p.Path)
	}

	// The dataclass rows come first: the TypedDict rows below may reference
	// them (the mailbox rows reference the mail row).
	var classRows, dictRows []restapiapi.Row
	for _, r := range cat.Rows {
		if pythonClassRows[r.Name] {
			classRows = append(classRows, r)
			continue
		}
		dictRows = append(dictRows, r)
	}
	for _, r := range classRows {
		b.WriteString("\n\n")
		pythonDataclass(&b, r)
	}
	for _, r := range dictRows {
		b.WriteString("\n\n")
		pythonTypedDict(&b, r)
	}
	return b.Bytes()
}

// pythonHeader writes the generated file's docstring and imports.
func pythonHeader(b *bytes.Buffer) {
	b.WriteString(pythonPreamble + "\n" +
		"from __future__ import annotations\n" +
		"\n" +
		"from dataclasses import dataclass, field\n" +
		"from typing import Any, Dict, List, TypedDict\n")
}

// pythonPreamble is the generated Python file's docstring.
const pythonPreamble = `"""Generated by 'go run ./tools/sdkgen' from kernel/contract/restapiapi.

DO NOT EDIT: regenerate with 'go run ./tools/sdkgen'.

The daemon's /api/v1 contract as Python: the paths the SDK calls and the
wire rows the daemon answers with. The client in client.py uses these
names, so a path or a field exists in exactly one place.
"""`

// pythonDataclass renders one row as a dataclass with a tolerant parser.
func pythonDataclass(b *bytes.Buffer, r restapiapi.Row) {
	fmt.Fprintf(b, "@dataclass\nclass %s:\n", pascal(r.Name))
	fmt.Fprintf(b, "    \"\"\"%s\"\"\"\n\n", r.Doc)
	for _, f := range r.Fields {
		fmt.Fprintf(b, "    %s: %s = %s\n", f.LocalName("python"), pythonType(f.Type), pythonDefault(f.Type))
	}
	b.WriteString("\n    @classmethod\n")
	fmt.Fprintf(b, "    def from_dict(cls, d: Dict[str, Any]) -> %q:\n", pascal(r.Name))
	fmt.Fprintf(b, "        return cls(\n")
	for _, f := range r.Fields {
		fmt.Fprintf(b, "            %s=%s,\n", f.LocalName("python"), pythonRead(f))
	}
	b.WriteString("        )\n")
}

// pythonTypedDict renders one row as a TypedDict.
func pythonTypedDict(b *bytes.Buffer, r restapiapi.Row) {
	total := "total=True"
	for _, f := range r.Fields {
		if !f.Required {
			total = "total=False"
			break
		}
	}
	if total == "total=True" {
		fmt.Fprintf(b, "class %s(TypedDict):\n", pascal(r.Name))
	} else {
		fmt.Fprintf(b, "class %s(TypedDict, total=False):\n", pascal(r.Name))
	}
	fmt.Fprintf(b, "    \"\"\"%s\"\"\"\n\n", r.Doc)
	for _, f := range r.Fields {
		fmt.Fprintf(b, "    %s: %s\n", f.LocalName("python"), pythonType(f.Type))
	}
}

// pythonType maps a wire type onto its Python spelling.
func pythonType(t restapiapi.Type) string {
	switch t {
	case restapiapi.TypeString:
		return "str"
	case restapiapi.TypeInt:
		return "int"
	case restapiapi.TypeBool:
		return "bool"
	case restapiapi.TypeStringSlice:
		return "List[str]"
	case restapiapi.TypeAnySlice:
		return "List[Any]"
	case restapiapi.TypeAnyObject:
		return "Dict[str, Any]"
	case restapiapi.TypeIntMap:
		return "Dict[str, int]"
	case restapiapi.TypeMail:
		return "Mail"
	case restapiapi.TypeMailSlice:
		return "List[Mail]"
	default:
		return "Any"
	}
}

// pythonDefault maps a wire type onto a dataclass default.
func pythonDefault(t restapiapi.Type) string {
	switch t {
	case restapiapi.TypeString:
		return `""`
	case restapiapi.TypeInt:
		return "0"
	case restapiapi.TypeBool:
		return "False"
	case restapiapi.TypeStringSlice, restapiapi.TypeAnySlice:
		return "field(default_factory=list)"
	case restapiapi.TypeAnyObject, restapiapi.TypeIntMap:
		return "field(default_factory=dict)"
	default:
		return "None"
	}
}

// pythonRead renders the read of one field inside from_dict.
func pythonRead(f restapiapi.Field) string {
	switch f.Type {
	case restapiapi.TypeString:
		return fmt.Sprintf("str(d.get(%q) or %q)", f.Name, "")
	case restapiapi.TypeInt:
		return fmt.Sprintf("int(d.get(%q) or 0)", f.Name)
	case restapiapi.TypeBool:
		return fmt.Sprintf("bool(d.get(%q) or False)", f.Name)
	default:
		return fmt.Sprintf("d.get(%q)", f.Name)
	}
}

// --- TypeScript ------------------------------------------------------------

// renderTypeScript renders the TypeScript SDK's catalog.
func renderTypeScript(cat catalog) []byte {
	var b bytes.Buffer
	b.WriteString("// Generated by 'go run ./tools/sdkgen' from kernel/contract/restapiapi.\n" +
		"//\n" +
		"// DO NOT EDIT: regenerate with 'go run ./tools/sdkgen'.\n" +
		"//\n" +
		"// The daemon's /api/v1 contract as TypeScript: the paths the SDK calls and\n" +
		"// the wire rows the daemon answers with.\n" +
		"\n" +
		"/** The daemon's REST paths. */\n" +
		"export const PATHS = {\n")
	for _, p := range cat.Paths {
		fmt.Fprintf(&b, "  %s: %q,\n", camel(p.Name), p.Path)
	}
	b.WriteString("} as const;\n")
	for _, r := range cat.Rows {
		b.WriteString("\n")
		fmt.Fprintf(&b, "/** %s */\n", r.Doc)
		fmt.Fprintf(&b, "export interface %s {\n", pascal(r.Name))
		for _, f := range r.Fields {
			opt := ""
			if !f.Required {
				opt = "?"
			}
			fmt.Fprintf(&b, "  %s%s: %s;\n", f.LocalName("typescript"), opt, tsType(f.Type))
		}
		b.WriteString("}\n")
	}
	return b.Bytes()
}

// tsType maps a wire type onto its TypeScript spelling.
func tsType(t restapiapi.Type) string {
	switch t {
	case restapiapi.TypeString:
		return "string"
	case restapiapi.TypeInt:
		return "number"
	case restapiapi.TypeBool:
		return "boolean"
	case restapiapi.TypeStringSlice:
		return "string[]"
	case restapiapi.TypeAnySlice:
		return "unknown[]"
	case restapiapi.TypeAnyObject:
		return "Record<string, unknown>"
	case restapiapi.TypeIntMap:
		return "Record<string, number>"
	case restapiapi.TypeMail:
		return "Mail"
	case restapiapi.TypeMailSlice:
		return "Mail[]"
	default:
		return "unknown"
	}
}

// --- Go --------------------------------------------------------------------

// renderGo renders the Go SDK's catalog.
//
// The Go SDK speaks the control plane, not REST: it has no REST client and no
// REST paths. What it shares with the other SDKs is the board's wire
// vocabulary — the field names of a message and of a message to send — and
// those have an owner: the app layer's board rows. The constants are derived
// from those structs' json tags, so a renamed field changes them with it.
func renderGo(cat catalog) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by tools/sdkgen. DO NOT EDIT.\n" +
		"\n" +
		"package sdk\n" +
		"\n")

	b.WriteString("// The board's mail row: the wire field names the daemon sends and accepts.\n" +
		"//\n" +
		"// The Go SDK talks the control plane, whose board view carries the same wire\n" +
		"// names as the REST mailbox view, so both surfaces read one vocabulary.\n" +
		"const (\n")
	for _, f := range wireFields(reflect.TypeOf(board.Message{})) {
		fmt.Fprintf(&b, "\t%s = %q\n", goFieldConst("mailField", f.GoName), f.Wire)
	}
	b.WriteString(")\n")

	b.WriteString("\n// The board's mail-draft row: the field names of a message to send.\n" +
		"const (\n")
	for _, f := range wireFields(reflect.TypeOf(board.SendInput{})) {
		fmt.Fprintf(&b, "\t%s = %q\n", goFieldConst("mailDraftField", f.GoName), f.Wire)
	}
	b.WriteString(")\n")

	// gofmt is part of the contract of a generated Go file: the repository's
	// format gate must not depend on how the template happens to be spaced.
	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("sdkgen: the generated Go file does not parse: %w", err)
	}
	return out, nil
}

// wireField is one field of a wire row.
type wireField struct {
	// GoName is the struct's field name.
	GoName string
	// Wire is the field's json tag — the name on the wire.
	Wire string
}

// wireFields reads the json tags of a struct: the wire names of a row.
func wireFields(t reflect.Type) []wireField {
	var out []wireField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, wireField{GoName: f.Name, Wire: name})
	}
	return out
}

// goFieldConst turns a wire field name into a Go constant name
// ("ts_unix_ms" -> "TSUnixMS").
func goFieldConst(prefix, name string) string {
	return prefix + name
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
