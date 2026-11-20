// SPDX-License-Identifier: MIT

package opapi

// The route catalogue's own guard. The catalogue is the console's route
// table, so a mistake in it is a mistake in what the console serves: a
// duplicated path hides an operation, a method that contradicts the kind
// breaks the browser's contract, and an operation that is not in the
// vocabulary is a route the daemon will never answer.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// declaredOperationNames reads the operation names this package declares, so
// the catalogue can be checked against the vocabulary it belongs to.
func declaredOperationNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") || !strings.HasPrefix(entry.Name(), "names") {
			continue
		}
		parsed, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				values, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range values.Names {
					if i >= len(values.Values) {
						continue
					}
					literal, ok := values.Values[i].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					names[strings.Trim(literal.Value, `"`)] = true
					_ = name
				}
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("no operation names found in the package")
	}
	return names
}

// TestConsoleRoutesBindDeclaredOperations: every route binds an operation the
// vocabulary declares. A route to an undeclared name is a route the daemon
// cannot answer.
func TestConsoleRoutesBindDeclaredOperations(t *testing.T) {
	names := declaredOperationNames(t)
	for _, route := range ConsoleRoutes() {
		if route.Op == "" {
			t.Errorf("%s binds no operation", route.Path)
			continue
		}
		if !names[route.Op] {
			t.Errorf("%s binds %q, which is not a declared operation", route.Path, route.Op)
		}
	}
}

// TestConsoleRoutePathsAreUnique: two operations cannot answer at one path —
// one of them would be unreachable.
func TestConsoleRoutePathsAreUnique(t *testing.T) {
	paths := map[string]string{}
	for _, route := range ConsoleRoutes() {
		if previous, taken := paths[route.Path]; taken {
			t.Errorf("%s is bound twice: %s and %s", route.Path, previous, route.Op)
			continue
		}
		paths[route.Path] = route.Op
	}
}

// TestConsoleRouteMethodMatchesKind: the kind decides the method, and the
// console binds the proxy the kind implies. A GET that mutates (or a POST
// that only reads) is a security bug, not a style choice.
func TestConsoleRouteMethodMatchesKind(t *testing.T) {
	for _, route := range ConsoleRoutes() {
		switch route.Kind {
		case HTTPRouteRead, HTTPRouteReadArgs:
			if route.Method != "GET" {
				t.Errorf("%s is a read but answers %s", route.Path, route.Method)
			}
		case HTTPRouteWrite, HTTPRouteJSON, HTTPRouteStream:
			if route.Method != "POST" {
				t.Errorf("%s is a mutation but answers %s", route.Path, route.Method)
			}
		default:
			t.Errorf("%s has an unknown kind %d", route.Path, route.Kind)
		}
	}
}

// TestConsoleRouteArgumentAllowlistsAreClean: a parameterless read forwards
// nothing, and no allowlist names the same argument twice.
func TestConsoleRouteArgumentAllowlistsAreClean(t *testing.T) {
	for _, route := range ConsoleRoutes() {
		if route.Kind == HTTPRouteRead && len(route.Args) > 0 {
			t.Errorf("%s is a parameterless read but forwards %v", route.Path, route.Args)
		}
		seen := map[string]bool{}
		for _, arg := range route.Args {
			if arg == "" {
				t.Errorf("%s forwards an empty argument name", route.Path)
				continue
			}
			if seen[arg] {
				t.Errorf("%s forwards %q twice", route.Path, arg)
			}
			seen[arg] = true
		}
	}
}

// TestConsoleRouteIndexAnswersEveryRoute: the index the console reads is the
// same table the catalogue declares.
func TestConsoleRouteIndexAnswersEveryRoute(t *testing.T) {
	for _, route := range ConsoleRoutes() {
		indexed, ok := ConsoleRoute(route.Op)
		if !ok {
			t.Errorf("%s is not in the index", route.Op)
			continue
		}
		if indexed.Path != route.Path || indexed.Method != route.Method || indexed.Kind != route.Kind {
			t.Errorf("%s is indexed as %+v, declared as %+v", route.Op, indexed, route)
		}
		if len(ConsoleArgs(route.Op)) != len(route.Args) {
			t.Errorf("%s forwards %d arguments, declared %d", route.Op, len(ConsoleArgs(route.Op)), len(route.Args))
		}
	}
}
