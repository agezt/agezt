// SPDX-License-Identifier: MIT

package webui

// The console's operation arguments are the registry's request rows.
//
// The operation registry says what every operation ACCEPTS: its input schema
// names the fields, and its "required" list names the ones that must arrive.
// The console says what it SENDS: the route catalogue's forwarded argument
// list, and the argument maps the console's own handlers used to spell out by
// hand.
//
// Two lists of the same fields are two homes for one row, and a home that
// drifts is silent. An argument the console forwards that the operation never
// declared is a field nobody promised to read (the daemon tolerates it only
// because its schema allows unknown input), and a required field the console
// never sends is a call that fails at runtime.
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
// And since the rows moved into the contract (kernel/contract/opapi), the
// console's own spelling is now a VIOLATION rather than a thing to compare:
// a hand-written argument map is a second spelling of a row that already has
// a home. Three rules hold that:
//
//   1. an operation call must pass the operation's row through opapi.Args —
//      a hand-written map, direct or through a variable, is refused;
//   2. the row a call passes must be the operation's OWN row;
//   3. the registry's declared input for an operation with a contract row must
//      BE the schema derived from that row — so the published contract and
//      the row cannot drift.
//
// The checks are the console's own: they read the registry at build time (the
// operations are bound in package initialisers, so no daemon is needed to ask
// what an operation accepts) and they read the console's own sources with
// go/parser, so the calls are read as syntax, not as text.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/platform/schema"
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

// consoleTypeAliases reads the console's own type aliases — the body's names
// that ARE a contract row, so the row is written once — and returns the row's
// name each alias stands for.
//
// The console's body structs are aliases of the contract's rows
// (`fileMkdirBody = opapi.FileMkdirRequest`), so a call that forwards the body
// forwards the row. The guard has to see through the alias, or the call would
// look like it forwards nothing.
func consoleTypeAliases(t *testing.T) map[string]string {
	t.Helper()
	aliases := map[string]string{}
	for _, path := range consoleSourceFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("the console's %s is unreadable: %v", filepath.Base(path), err)
		}
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Assign.IsValid() {
					continue // a declared type is not an alias
				}
				switch shape := typeSpec.Type.(type) {
				case *ast.SelectorExpr:
					if pkg, ok := shape.X.(*ast.Ident); ok && pkg.Name == "opapi" {
						aliases[typeSpec.Name.Name] = shape.Sel.Name
					}
				case *ast.Ident:
					aliases[typeSpec.Name.Name] = shape.Name
				}
			}
		}
	}
	return aliases
}

// operationCall is one console call that names an operation.
type operationCall struct {
	// File is the console source the call lives in.
	File string
	// Operation is the operation's name.
	Operation string
	// Row is the request row the call passes, by name — empty when the call
	// passes no row at all.
	Row string
	// HandWritten is true when the call spells its arguments out as a map
	// literal instead of passing a row.
	HandWritten bool
	// Unreadable is true when the call forwards something the guard cannot
	// read as a row — a row it must refuse rather than skip.
	Unreadable bool
}

// localVarTypes reads the local variable declarations of one function: the
// name of a variable and the type it holds.
//
// The console's file handlers read the browser's body into a variable whose
// type IS the operation's row (the body's struct is an ALIAS of it), so the
// guard has to know what the variable holds to know which row is forwarded.
func localVarTypes(body *ast.BlockStmt) map[string]string {
	vars := map[string]string{}
	ast.Inspect(body, func(node ast.Node) bool {
		decl, ok := node.(*ast.GenDecl)
		if !ok || decl.Tok != token.VAR {
			return true
		}
		for _, spec := range decl.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Type == nil {
				continue
			}
			switch shape := value.Type.(type) {
			case *ast.Ident:
				vars[value.Names[0].Name] = shape.Name
			case *ast.SelectorExpr:
				if pkg, ok := shape.X.(*ast.Ident); ok && pkg.Name == "opapi" {
					vars[value.Names[0].Name] = shape.Sel.Name
				}
			}
		}
		return true
	})
	return vars
}

// operationCalls finds the console's calls that name an operation.
//
// The console has two kinds of operation calls, and they are told apart by
// what they forward:
//
//   - a call that BUILDS an operation's request forwards the operation's own
//     row — the console's ten typed calls;
//   - a call that RELAYS what the browser sent forwards the catalogue's
//     declared arguments (the streaming proxies), and names no row at all.
//
// The second kind is not a second spelling of a row: the catalogue says what
// those arguments are, and W5.33's guard holds the catalogue to the registry.
func operationCalls(t *testing.T) []operationCall {
	t.Helper()
	constants := operationConstants(t)
	aliases := consoleTypeAliases(t)
	calls := []operationCall{}
	for _, path := range consoleSourceFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("the console's %s is unreadable: %v", filepath.Base(path), err)
		}
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			vars := localVarTypes(function.Body)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Stream" {
					// A streaming proxy relays the catalogue's arguments.
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
					found := operationCall{File: filepath.Base(path), Operation: name}
					switch next := call.Args[i+1].(type) {
					case *ast.CompositeLit:
						// A hand-written argument map: a second spelling of a row
						// that already has a home.
						found.HandWritten = true
					case *ast.Ident:
						// A hand-written argument map through a variable: the same
						// second spelling, one step removed.
						found.HandWritten = true
					case *ast.CallExpr:
						// opapi.Args(row) — the row the console forwards.
						if len(next.Args) != 1 {
							continue
						}
						switch argument := next.Args[0].(type) {
						case *ast.CompositeLit:
							switch shape := argument.Type.(type) {
							case *ast.Ident:
								found.Row = shape.Name
							case *ast.SelectorExpr:
								// A qualified row type: opapi.ArtifactGetRequest.
								found.Row = shape.Sel.Name
							default:
								found.Unreadable = true
							}
						case *ast.Ident:
							// The console's own name for the row: the body struct
							// is an ALIAS of the contract's row.
							held, ok := vars[argument.Name]
							if !ok {
								found.Unreadable = true
								break
							}
							if row, ok := aliases[held]; ok {
								found.Row = row
								break
							}
							found.Row = held
						default:
							found.Unreadable = true
						}
					default:
						found.Unreadable = true
					}
					calls = append(calls, found)
				}
				return true
			})
		}
	}
	return calls
}

// rowNameOf returns the name of the request row the contract binds to one
// operation.
func rowNameOf(operation string) string {
	for _, binding := range opapi.RequestRows {
		if binding.Operation == operation {
			return reflect.TypeOf(binding.Row).Name()
		}
	}
	return ""
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

// TestTheConsolePassesTheOperationsOwnRow is the ratchet: an operation call
// must pass the operation's request row through opapi.Args. A hand-written
// argument map — direct or through a variable — is a second spelling of a row
// that already has a home, and a row paired with the wrong operation is a
// promise the operation never made.
func TestTheConsolePassesTheOperationsOwnRow(t *testing.T) {
	calls := operationCalls(t)
	if len(calls) == 0 {
		t.Fatal("the console's operation calls were not found — the guard measures nothing")
	}
	for _, call := range calls {
		if call.Unreadable {
			t.Errorf("%s forwards %s something the guard cannot read as a request row — a call the guard cannot see is a call it cannot hold",
				call.File, call.Operation)
			continue
		}
		if call.HandWritten {
			t.Errorf("%s spells the arguments of %s by hand — the operation's request row lives in the contract, and it is written once",
				call.File, call.Operation)
			continue
		}
		if call.Row == "" {
			t.Errorf("%s calls %s without a request row — the operation's request row lives in the contract",
				call.File, call.Operation)
			continue
		}
		want := rowNameOf(call.Operation)
		if want == "" {
			t.Errorf("%s calls %s, which the contract names no request row for", call.File, call.Operation)
			continue
		}
		if call.Row != want {
			t.Errorf("%s calls %s with the %s row — the operation's own row is %s",
				call.File, call.Operation, call.Row, want)
		}
	}
	// The contract's rows are the operations' OWN rows — whether the console
	// calls them with a typed row (its own ten) or through the catalogue's
	// proxy (which forwards the catalogue's declared arguments). So the row
	// table is not the console's list: the third rule below is what holds
	// every row to the registry.
	for _, binding := range opapi.RequestRows {
		if rowNameOf(binding.Operation) == "" {
			t.Errorf("the contract's row for %s is not reachable", binding.Operation)
		}
	}
}

// TestTheRegistrysDeclaredInputIsTheRow is the third rule: the schema the
// registry declares for an operation with a contract row must BE the schema
// derived from that row. A hand-written schema beside the row is a second
// spelling of the same shape, and it can drift.
func TestTheRegistrysDeclaredInputIsTheRow(t *testing.T) {
	schemas := controlplane.AppOperationSchemas()
	for _, binding := range opapi.RequestRows {
		declared, ok := schemas[binding.Operation]
		if !ok {
			t.Errorf("the registry does not declare the %s operation", binding.Operation)
			continue
		}
		same, err := declaredInputIsTheRow(declared.Input, binding.Row)
		if err != nil {
			t.Errorf("the %s request row does not derive a schema: %v", binding.Operation, err)
			continue
		}
		if !same {
			derived, _ := schema.FromType(reflect.TypeOf(binding.Row), true)
			t.Errorf("the %s operation declares a schema of its own:\n declared: %s\n derived:  %s",
				binding.Operation, declared.Input, derived)
		}
	}
}

// declaredInputIsTheRow reports whether a declared input schema IS the schema
// derived from the operation's request row.
//
// It is apart from the test body so that the guard ITSELF can be measured: a
// comparison that quietly stops comparing must fail the guard's own test.
func declaredInputIsTheRow(declared json.RawMessage, row any) (bool, error) {
	derived, err := schema.FromType(reflect.TypeOf(row), true)
	if err != nil {
		return false, err
	}
	return sameSchema(declared, derived), nil
}

// sameSchema reports whether two schemas say the same thing.
func sameSchema(a, b json.RawMessage) bool {
	var left, right any
	if err := json.Unmarshal(a, &left); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	leftText, _ := json.Marshal(left)
	rightText, _ := json.Marshal(right)
	return string(leftText) == string(rightText)
}

// TestTheGuardSeesASchemaOfItsOwn is the guard's own test for the third
// rule: the guard must see a declared schema that is NOT the row's. A guard
// that cannot compare would pass silently while the published contract
// drifted away from the row.
func TestTheGuardSeesASchemaOfItsOwn(t *testing.T) {
	row := opapi.ConfigSetRequest{}
	derived, err := schema.FromType(reflect.TypeOf(row), true)
	if err != nil {
		t.Fatal(err)
	}
	same, err := declaredInputIsTheRow(derived, row)
	if err != nil || !same {
		t.Errorf("the row's own schema must read as the row: %v %v", same, err)
	}
	other := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"value":{"type":"string"},"section":{"type":"object"}}}`)
	same, err = declaredInputIsTheRow(other, row)
	if err != nil {
		t.Fatal(err)
	}
	if same {
		t.Error("a schema that is not the row's must be seen as a schema of its own")
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

// TestTheGuardSeesASecondSpelling is the guard's own test for the row rules:
// the guard must see a hand-written argument map, and it must see the row a
// call passes. A guard that cannot read the console's own calls would pass
// silently while the console drifts.
func TestTheGuardSeesASecondSpelling(t *testing.T) {
	calls := operationCalls(t)
	rows := 0
	for _, call := range calls {
		if call.HandWritten {
			t.Errorf("the console spells an operation's arguments by hand: %s calls %s with a map literal",
				call.File, call.Operation)
		}
		if call.Unreadable {
			t.Errorf("the guard cannot read the row %s forwards to %s", call.File, call.Operation)
		}
		if call.Row != "" {
			rows++
		}
	}
	if rows == 0 {
		t.Error("the guard sees no call that passes a request row — it is not reading the console's calls")
	}
	if rows != len(calls) {
		t.Errorf("the guard reads a request row in %d of %d calls — a call it cannot read is a call it cannot hold",
			rows, len(calls))
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
