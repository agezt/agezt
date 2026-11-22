// SPDX-License-Identifier: MIT

package opapi

import (
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
