// SPDX-License-Identifier: MIT

package opapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestTheConsoleNativeListIsWellFormed: the console's own endpoint list is a
// promise with four parts per entry — a name, a path, a method and a tier —
// so the list itself has to be well formed: no two endpoints may answer at the
// same path, no two may share a name, and every method must be one the
// console's router understands.
func TestTheConsoleNativeListIsWellFormed(t *testing.T) {
	names := map[NativeRouteName]bool{}
	paths := map[string]bool{}
	for _, route := range ConsoleNativeRoutes() {
		if route.Name == "" {
			t.Errorf("the native endpoint %q has no name", route.Path)
		}
		if names[route.Name] {
			t.Errorf("two console-native endpoints share the name %q", route.Name)
		}
		names[route.Name] = true
		if route.Path == "" {
			t.Errorf("the console-native endpoint %q has no path", route.Name)
		}
		if paths[route.Path] {
			t.Errorf("two console-native endpoints answer at %q", route.Path)
		}
		paths[route.Path] = true
		if !nativeMethodIsKnown(route.Method) {
			t.Errorf("the console-native endpoint %q declares the method %q, which the router does not understand",
				route.Name, route.Method)
		}
	}
}

// nativeMethodIsKnown reports whether the router can enforce the method: a
// comma-separated list of HTTP verbs, or the single wildcard.
func nativeMethodIsKnown(method string) bool {
	if method == "" {
		return false
	}
	for _, candidate := range strings.Split(method, ",") {
		verb := strings.ToUpper(strings.TrimSpace(candidate))
		switch verb {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		case "*":
			if method != "*" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// TestConsoleNativePathIsTheRoutesPath: the lookup the console uses to name
// its own endpoints answers the path the list states.
func TestConsoleNativePathIsTheRoutesPath(t *testing.T) {
	for _, route := range ConsoleNativeRoutes() {
		if got := ConsoleNativePath(route.Name); got != route.Path {
			t.Errorf("ConsoleNativePath(%q) = %q, want %q", route.Name, got, route.Path)
		}
	}
}

// TestConsoleNativeWritesAreThePostOnes: the generated client splits the
// console-native paths by verb, so the question "is this endpoint a write?"
// must have exactly one answer. A route with no POST verb is a read; a route
// with one is a write.
func TestConsoleNativeWritesAreThePostOnes(t *testing.T) {
	for _, route := range ConsoleNativeRoutes() {
		want := strings.Contains(strings.ToUpper(route.Method), http.MethodPost)
		if got := route.IsWrite(); got != want {
			t.Errorf("the console-native endpoint %q answers %s, but IsWrite() says %v", route.Name, route.Method, got)
		}
	}
}

// TestTheConsoleNativeBodiesAreTypedSchemas holds the console's own request
// side: a body the console's endpoint list names must be a schema the door
// can CHECK.
//
// Three rules, and each one is a rule the door has:
//
//   - every property declares a TYPE — a property with no type is "anything",
//     and the door would accept anything for it;
//   - the schema carries no `required` list — the registry's door checks
//     TYPES, and presence is the handler's business;
//   - an undeclared field stays allowed — a schema is the stable SUBSET of a
//     body, so the daemon may read more than the contract names.
//
// It walks the whole list, so an endpoint added later is covered without this
// test knowing its name.
func TestTheConsoleNativeBodiesAreTypedSchemas(t *testing.T) {
	for _, route := range ConsoleNativeRoutes() {
		if len(route.Body) == 0 {
			continue
		}
		var doc struct {
			Type                 string                     `json:"type"`
			AdditionalProperties bool                       `json:"additionalProperties"`
			Required             []string                   `json:"required"`
			Properties           map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(route.Body, &doc); err != nil {
			t.Errorf("the console-native endpoint %q declares an unreadable body schema: %v", route.Name, err)
			continue
		}
		if doc.Type != "object" {
			t.Errorf("the console-native endpoint %q declares the %q body schema, want object", route.Name, doc.Type)
		}
		if len(doc.Required) != 0 {
			t.Errorf("the console-native endpoint %q declares the required list %v — the door checks types, not presence",
				route.Name, doc.Required)
		}
		if !doc.AdditionalProperties {
			t.Errorf("the console-native endpoint %q does not allow an undeclared field — a schema is the stable subset of a body",
				route.Name)
		}
		if len(doc.Properties) == 0 {
			t.Errorf("the console-native endpoint %q declares a body schema with no property", route.Name)
		}
		for name, raw := range doc.Properties {
			var prop struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &prop); err != nil {
				t.Errorf("the console-native endpoint %q declares an unreadable %q property: %v", route.Name, name, err)
				continue
			}
			if prop.Type == "" {
				t.Errorf("the console-native endpoint %q declares %q with no type — the door would accept anything for it",
					route.Name, name)
			}
		}
	}
}

// TestTheConsoleNativeBodyLookupAnswersTheList: the lookup the console's door
// uses answers the schema the list states, and an endpoint that takes no body
// answers false.
func TestTheConsoleNativeBodyLookupAnswersTheList(t *testing.T) {
	for _, route := range ConsoleNativeRoutes() {
		shape, ok := ConsoleNativeBodySchema(route.Name)
		if len(route.Body) == 0 {
			if ok {
				t.Errorf("the console-native endpoint %q takes no body, but the lookup answers one", route.Name)
			}
			continue
		}
		if !ok {
			t.Errorf("the console-native endpoint %q declares a body, but the lookup answers none", route.Name)
			continue
		}
		if string(shape) != string(route.Body) {
			t.Errorf("the console-native endpoint %q answers the body %s, want %s", route.Name, shape, route.Body)
		}
	}
}
