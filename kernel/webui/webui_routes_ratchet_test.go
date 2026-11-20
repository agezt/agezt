// SPDX-License-Identifier: MIT

package webui

// The console's route guard. The console's routes are DERIVED from the
// operation catalogue (kernel/contract/opapi/http.go): the catalogue says
// which path answers which operation, and the console binds a proxy for each.
// These tests hold that line: the console serves the catalogue and nothing
// else, and no file in the console declares a path of its own.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// consoleNativePaths are the console's own endpoints. They are not operation
// bindings — they serve the SPA shell, the auth surface, the event feed, the
// file manager, the voice surface, the rollback surface and the workflow hook
// — so they are registered one by one instead of coming from the catalogue.
var consoleNativePaths = []string{
	"/",
	"/api/authmeta",
	"/api/login",
	"/api/logout",
	"/assets/",
	"/favicon.ico",
	"/events",
	"/api/rollback/checkpoints",
	"/api/rollback/apply",
	"/api/sse-token",
	"/api/voice/status",
	"/api/transcribe",
	"/api/tts",
	"/api/artifact/raw",
	"/api/files/tree",
	"/api/files/raw",
	"/api/files/mkdir",
	"/api/files/rename",
	"/api/files/delete",
	"/hooks/",
	"/oauth/callback",
}

// allowedConsolePaths is every path the console may answer at: the
// catalogue's, plus the console's own.
func allowedConsolePaths() map[string]bool {
	allowed := map[string]bool{}
	for _, path := range consoleRoutePaths() {
		allowed[path] = true
	}
	for _, path := range consoleNativePaths {
		allowed[path] = true
	}
	return allowed
}

// TestConsoleRegistersTheCatalogueAndNothingElse: the console's router is the
// catalogue plus the console's own endpoints — every catalogue route is
// registered, none is registered twice, and nothing else is.
func TestConsoleRegistersTheCatalogueAndNothingElse(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	routes := s.routeRegistry().Routes()
	allowed := allowedConsolePaths()
	registered := map[string]bool{}
	for _, route := range routes {
		if registered[route.Path] {
			t.Errorf("%s is registered twice", route.Path)
			continue
		}
		registered[route.Path] = true
		if !allowed[route.Path] {
			t.Errorf("%s is served but is neither a catalogue route nor a console-native endpoint", route.Path)
		}
	}
	for _, path := range consoleRoutePaths() {
		if !registered[path] {
			t.Errorf("%s is in the catalogue but the console does not serve it", path)
		}
	}
}

// TestConsolePathLiteralsLiveInTheCatalogue: no file in the console may
// declare a path of its own. A path written in the console is a second copy
// of the catalogue — the thing this slice removed.
func TestConsolePathLiteralsLiveInTheCatalogue(t *testing.T) {
	allowed := allowedConsolePaths()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			path := strings.Trim(literal.Value, `"`)
			if !looksLikeConsolePath(path) {
				return true
			}
			if !allowed[path] {
				t.Errorf("%s declares the console path %q, which is neither a catalogue route nor a console-native endpoint",
					filepath.Join("kernel", "webui", entry.Name()), path)
			}
			return true
		})
	}
}

// looksLikeConsolePath reports whether a literal is a console path at all.
func looksLikeConsolePath(path string) bool {
	for _, prefix := range []string{"/api", "/hooks", "/oauth", "/events", "/assets", "/favicon"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
