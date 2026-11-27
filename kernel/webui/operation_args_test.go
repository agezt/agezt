// SPDX-License-Identifier: MIT

package webui

// The console's operation arguments are the registry's request rows.
//
// The operation registry says what every operation ACCEPTS: its input schema
// names the fields, and its "required" list names the ones that must arrive.
// The console says what it SENDS: the route catalogue's forwarded argument
// list, and the argument maps the console's own handlers spell out by hand.
//
// Two lists of the same fields are two homes for one row, and a home that
// drifts is silent. An argument the console forwards that the operation never
// declared is a field nobody promised to read — the daemon tolerates it only
// because its schema allows unknown input. A field the operation requires that
// the console never sends is a call that fails at runtime, in production,
// under the operator.
//
// So the console is measured against the registry:
//
//   - every argument the console forwards must be declared by the operation's
//     request row;
//   - every field the operation requires must be sent;
//   - an argument the console consumes itself must NOT be one of the
//     operation's fields — that would be one row with two homes (the console's
//     own input and the operation's request).
//
// The check is the console's own: it reads the registry at build time (the
// operations are bound in package initialisers, so no daemon is needed to ask
// what an operation accepts) and it reads the console's own sources for the
// argument maps it spells by hand.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/controlplane"
)

// requestRow is one operation's declared request row: the fields it names, and
// the fields it requires.
type requestRow struct {
	Fields   map[string]bool
	Required []string
}

// requestRows reads the registry's declared request rows, keyed by operation
// name.
func requestRows(t *testing.T) map[string]requestRow {
	t.Helper()
	schemas := controlplane.AppOperationSchemas()
	rows := make(map[string]requestRow, len(schemas))
	for name, schema := range schemas {
		var decoded struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(schema.Input, &decoded); err != nil {
			t.Fatalf("the %s operation's request row is unreadable: %v", name, err)
		}
		fields := make(map[string]bool, len(decoded.Properties))
		for field := range decoded.Properties {
			fields[field] = true
		}
		rows[name] = requestRow{Fields: fields, Required: decoded.Required}
	}
	return rows
}

// consoleSourceFiles lists the console's own sources — the surface the guard
// measures.
func consoleSourceFiles(t *testing.T) []string {
	t.Helper()
	dir := packageDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("the console's sources are unreadable: %v", err)
	}
	paths := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths
}

// operationConstants reads the operation names behind the opapi constants, by
// reading the contract's own source. The constants are untyped string
// constants — the operation names themselves — so their values are the
// operations' names.
func operationConstants(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(packageDir(t), "..", "contract", "opapi")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("the operation names are unreadable: %v", err)
	}
	fset := token.NewFileSet()
	names := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("the operation names in %s are unreadable: %v", name, err)
		}
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || value.Type != nil {
					continue // a typed constant is not an operation name
				}
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					literal, ok := value.Values[i].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					names[name.Name] = strings.Trim(literal.Value, `"`)
				}
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("the operation names are empty — the contract's constants were not found")
	}
	return names
}

// operationCall is one console call that names an operation and spells its
// arguments out by hand.
type operationCall struct {
	File string
	Op   string
	Keys []string
}

// handWrittenCalls finds the console's calls that name an operation and
// spell their arguments out by hand — the argument maps the console writes
// itself instead of deriving them from the route catalogue.
func handWrittenCalls(t *testing.T) []operationCall {
	t.Helper()
	constants := operationConstants(t)
	calls := []operationCall{}
	for _, path := range consoleSourceFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("the console's %s is unreadable: %v", filepath.Base(path), err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			for i, argument := range call.Args {
				selector, ok := argument.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				pkg, ok := selector.X.(*ast.Ident)
				if !ok || pkg.Name != "opapi" {
					continue
				}
				name, ok := constants[selector.Sel.Name]
				if !ok {
					continue
				}
				if i+1 >= len(call.Args) {
					continue
				}
				literal, ok := call.Args[i+1].(*ast.CompositeLit)
				if !ok {
					continue
				}
				keys, ok := literalKeys(literal)
				if !ok {
					continue
				}
				calls = append(calls, operationCall{File: filepath.Base(path), Op: name, Keys: keys})
			}
			return true
		})
	}
	return calls
}

// literalKeys reads the keys of a hand-written map literal.
func literalKeys(literal *ast.CompositeLit) ([]string, bool) {
	mapType, ok := literal.Type.(*ast.MapType)
	if !ok {
		return nil, false
	}
	key, ok := mapType.Key.(*ast.Ident)
	if !ok || key.Name != "string" {
		return nil, false
	}
	keys := []string{}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return nil, false
		}
		name, ok := pair.Key.(*ast.BasicLit)
		if !ok || name.Kind != token.STRING {
			return nil, false
		}
		keys = append(keys, strings.Trim(name.Value, `"`))
	}
	sort.Strings(keys)
	return keys, true
}

// checkArguments compares the arguments a console call sends with the
// operation's declared request row, and returns the ways they disagree.
//
// It is the guard's core, kept apart from the test bodies so that the guard
// ITSELF can be measured: a check that is quietly dropped must fail the
// guard's own test, not pass unnoticed.
func checkArguments(row requestRow, arguments []string) []string {
	problems := []string{}
	for _, argument := range arguments {
		if !row.Fields[argument] {
			problems = append(problems, "forwards "+argument+", which the request row does not declare")
		}
	}
	for _, required := range row.Required {
		if !containsArgument(arguments, required) {
			problems = append(problems, "does not send "+required+", which the request row requires")
		}
	}
	return problems
}

// TestTheConsoleRoutesForwardOnlyDeclaredArguments checks the route
// catalogue: every argument a route forwards must be a field the operation's
// request row declares, every required field must be forwarded, and an
// argument the console consumes itself must not be one of the operation's
// fields.
func TestTheConsoleRoutesForwardOnlyDeclaredArguments(t *testing.T) {
	rows := requestRows(t)
	for _, route := range opapi.ConsoleRoutes() {
		row, ok := rows[route.Op]
		if !ok {
			t.Errorf("the console binds %s, which the registry does not declare", route.Op)
			continue
		}
		for _, problem := range checkArguments(row, route.Args) {
			t.Errorf("the %s route %s", route.Path, problem)
		}
		for _, consumed := range route.Consumed {
			if containsArgument(route.Args, consumed) {
				t.Errorf("the %s route both forwards and consumes %q — one argument, two homes",
					route.Path, consumed)
			}
			if row.Fields[consumed] {
				t.Errorf("the %s route consumes %q itself, but %q is a field of the %s request row — "+
					"the console's own input and the operation's request are one row with two homes",
					route.Path, consumed, consumed, route.Op)
			}
		}
	}
}

// TestTheConsoleSendsOnlyDeclaredArguments checks the console's own handlers:
// the argument maps they spell out by hand must name fields the operation's
// request row declares, and must send the fields it requires.
func TestTheConsoleSendsOnlyDeclaredArguments(t *testing.T) {
	rows := requestRows(t)
	calls := handWrittenCalls(t)
	if len(calls) == 0 {
		t.Fatal("the console's hand-written operation calls were not found — the guard measures nothing")
	}
	for _, call := range calls {
		row, ok := rows[call.Op]
		if !ok {
			t.Errorf("%s calls %s, which the registry does not declare", call.File, call.Op)
			continue
		}
		for _, problem := range checkArguments(row, call.Keys) {
			t.Errorf("%s %s", call.File, problem)
		}
	}
}

// TestTheGuardReportsBothWaysOfDrifting is the guard's own test: the check
// must report an argument the request row does not declare, AND a required
// field the call does not send. A guard that lost one of its two eyes must
// fail here.
func TestTheGuardReportsBothWaysOfDrifting(t *testing.T) {
	row := requestRow{
		Fields:   map[string]bool{"path": true, "parents": true},
		Required: []string{"path"},
	}
	problems := checkArguments(row, []string{"path", "paths"})
	if len(problems) != 1 || !strings.Contains(problems[0], "paths") {
		t.Errorf("an argument the request row does not declare must be reported: %v", problems)
	}
	problems = checkArguments(row, []string{"parents"})
	if len(problems) != 1 || !strings.Contains(problems[0], "path") {
		t.Errorf("a required field the call does not send must be reported: %v", problems)
	}
	if problems := checkArguments(row, []string{"path", "parents"}); len(problems) != 0 {
		t.Errorf("an argument list that matches the request row must be silent: %v", problems)
	}
}

// containsArgument reports whether the list names the argument.
func containsArgument(arguments []string, name string) bool {
	for _, argument := range arguments {
		if argument == name {
			return true
		}
	}
	return false
}
