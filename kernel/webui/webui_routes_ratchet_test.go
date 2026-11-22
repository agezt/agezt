// SPDX-License-Identifier: MIT

package webui

// The console's route guard. The console's routes are DERIVED from the
// operation catalogue (kernel/contract/opapi/http.go): the catalogue says
// which path answers which operation, and the console binds a proxy for each.
// Its OWN endpoints (kernel/contract/opapi/native.go) are derived the same
// way: the list carries the path, the method, the tier and the mutation flag,
// and the console binds a handler to each by NAME.
//
// These tests hold that line:
//   - the console serves the catalogue and the console's own endpoints, and
//     nothing else;
//   - every console-native endpoint is served with the method, the tier and
//     the mutation flag the contract states;
//   - every handler the console holds is named by the contract, and every
//     name the contract states has a handler;
//   - no file in the console declares a console path of its own.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// consoleNativeRoutes are the console's own endpoints as the CONTRACT states
// them: the path, the method, the tier and the mutation flag. They are not
// operation bindings — they serve the SPA shell, the auth surface, the event
// feed, the file manager, the voice surface, the rollback surface and the
// workflow hook — so they are not in the catalogue. They live in the contract
// package (opapi), next to the catalogue they complement: the generated
// TypeScript client publishes the console's path surface from the same two
// lists, and this guard reads the same source it checks.
var consoleNativeRoutes = opapi.ConsoleNativeRoutes()

// allowedConsolePaths is every path the console may answer at: the
// catalogue's, plus the console's own.
func allowedConsolePaths() map[string]bool {
	allowed := map[string]bool{}
	for _, path := range consoleRoutePaths() {
		allowed[path] = true
	}
	for _, route := range consoleNativeRoutes {
		allowed[route.Path] = true
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

// TestConsoleServesEveryNativeEndpoint: the console's own endpoint list is a
// promise in both directions. A name in the list that the console does not
// serve is a lie the generated client would publish; an endpoint the console
// serves that the list omits is a path nobody can check.
func TestConsoleServesEveryNativeEndpoint(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	registered := map[string]bool{}
	for _, route := range s.routeRegistry().Routes() {
		registered[route.Path] = true
	}
	for _, route := range consoleNativeRoutes {
		if !registered[route.Path] {
			t.Errorf("the console's own endpoint list names %q, but the console does not serve it", route.Path)
		}
	}
}

// TestConsoleNativeRoutesCarryTheContractPolicy: the console serves each of
// its own endpoints with the METHOD, the TIER and the MUTATION FLAG the
// contract states. A drift here is the whole failure this slice removes: the
// generated client publishes the contract's verb, and the console answers a
// different one — so the browser compiles against a promise the server breaks.
func TestConsoleNativeRoutesCarryTheContractPolicy(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	registered := map[string]httpserverRoute{}
	for _, route := range s.routeRegistry().Routes() {
		registered[route.Path] = httpserverRoute{
			method:   route.Method,
			tier:     route.Tier,
			mutation: route.Mutation,
		}
	}
	for _, route := range consoleNativeRoutes {
		actual, ok := registered[route.Path]
		if !ok {
			// Already reported by TestConsoleServesEveryNativeEndpoint.
			continue
		}
		if actual.method != route.Method {
			t.Errorf("the console answers %s with %s, but the contract says %s", route.Path, actual.method, route.Method)
		}
		wantTier := kernelauth.TierUser
		if route.Public {
			wantTier = kernelauth.TierPublic
		}
		if actual.tier != wantTier {
			t.Errorf("the console answers %s at tier %s, but the contract says %s", route.Path, actual.tier, wantTier)
		}
		if actual.mutation != route.Mutation {
			t.Errorf("the console answers %s with mutation=%v, but the contract says %v", route.Path, actual.mutation, route.Mutation)
		}
	}
}

// TestConsoleNativeHandlersAreNamedByTheContract: the console's handlers are
// keyed by the contract's NAME. A handler the contract does not name is a
// route nobody serves; a name the contract states with no handler is a route
// the console cannot answer. Both are caught here, and the second is caught
// at start-up too (the router panics rather than serving a 404).
func TestConsoleNativeHandlersAreNamedByTheContract(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	bindings := s.nativeBindings()
	named := map[opapi.NativeRouteName]bool{}
	for _, route := range consoleNativeRoutes {
		named[route.Name] = true
		if _, ok := bindings[route.Name]; !ok {
			t.Errorf("the contract names the console-native endpoint %q, but the console has no handler for it", route.Name)
		}
	}
	for name := range bindings {
		if !named[name] {
			t.Errorf("the console holds a handler for %q, which the contract does not name", name)
		}
	}
}

// TestConsoleDeclaresNoPathOfItsOwn: no file in the console may declare a
// console path of its own — not a catalogue route, not a console-native
// endpoint. Every path the console answers is read from the contract, so a
// path written in the console is a second copy of the contract, and the thing
// this programme has spent W3.82 and W5.1 removing.
//
// The console names its own endpoints by the contract's NAME, and it reads the
// catalogue's paths from the catalogue, so a literal here is always a copy.
func TestConsoleDeclaresNoPathOfItsOwn(t *testing.T) {
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
			t.Errorf("%s declares the console path %q; every path the console answers lives in the contract",
				filepath.Join("kernel", "webui", entry.Name()), path)
			return true
		})
	}
}

// httpserverRoute is the console's own snapshot of a registered route, kept
// small so the policy test reads as a policy.
type httpserverRoute struct {
	method   string
	tier     kernelauth.Tier
	mutation bool
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
