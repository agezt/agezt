// SPDX-License-Identifier: MIT

package restapiapi

import (
	"strings"
	"testing"
)

// rowByName finds a row in the contract. It lives in the test file on
// purpose: the contract's own lookup helpers would be production symbols no
// binary calls, and the dead-code gate would (rightly) report them.
func rowByName(t *testing.T, name string) Row {
	t.Helper()
	for _, r := range Rows() {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("the contract has no %q row", name)
	return Row{}
}

// routeByID finds a route in the contract.
func routeByID(t *testing.T, id string) Route {
	t.Helper()
	for _, r := range Routes() {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("the contract has no %q route", id)
	return Route{}
}

func TestRouteIDsAndPathsAreUnique(t *testing.T) {
	ids := map[string]bool{}
	paths := map[string]bool{}
	for _, r := range Routes() {
		if ids[r.ID] {
			t.Fatalf("duplicate route id %q", r.ID)
		}
		ids[r.ID] = true
		if paths[r.Path] {
			t.Fatalf("duplicate route path %q", r.Path)
		}
		paths[r.Path] = true
		if !strings.HasPrefix(r.Path, "/") {
			t.Fatalf("route %q has a relative path %q", r.ID, r.Path)
		}
		if len(r.Methods) == 0 {
			t.Fatalf("route %q serves no method", r.ID)
		}
		for _, m := range r.Methods {
			if m.Verb != "GET" && m.Verb != "POST" {
				t.Fatalf("route %q serves the unexpected verb %q", r.ID, m.Verb)
			}
		}
	}
}

func TestEndpointIDsAreUniqueAndNameTheirRows(t *testing.T) {
	ids := map[string]bool{}
	for _, e := range Endpoints() {
		if ids[e.ID] {
			t.Fatalf("duplicate endpoint id %q", e.ID)
		}
		ids[e.ID] = true
		if routeByID(t, e.Route).ID != e.Route {
			t.Fatalf("endpoint %q names the unknown route %q", e.ID, e.Route)
		}
		if e.Request != "" {
			if rowByName(t, e.Request).Name != e.Request {
				t.Fatalf("endpoint %q names the unknown request row %q", e.ID, e.Request)
			}
		}
		if e.Response != "" {
			if rowByName(t, e.Response).Name != e.Response {
				t.Fatalf("endpoint %q names the unknown response row %q", e.ID, e.Response)
			}
		}
		if !strings.HasPrefix(e.Path, "/api/v1") {
			t.Fatalf("endpoint %q has the non-SDK path %q", e.ID, e.Path)
		}
	}
}

func TestTheRowsAreUniqueAndWellFormed(t *testing.T) {
	names := map[string]bool{}
	for _, r := range Rows() {
		if names[r.Name] {
			t.Fatalf("duplicate row %q", r.Name)
		}
		names[r.Name] = true
		if len(r.Fields) == 0 {
			t.Fatalf("row %q has no field", r.Name)
		}
		fields := map[string]bool{}
		for _, f := range r.Fields {
			if fields[f.Name] {
				t.Fatalf("row %q has the duplicate field %q", r.Name, f.Name)
			}
			fields[f.Name] = true
			if f.Type == TypeMail || f.Type == TypeMailSlice {
				if rowByName(t, RowMail).Name != RowMail {
					t.Fatalf("row %q references the mail row, which does not exist", r.Name)
				}
			}
		}
	}
}

func TestEverySDKEndpointHasARowAndTheRestDoNot(t *testing.T) {
	for _, e := range Endpoints() {
		route := routeByID(t, e.Route)
		if e.SDK && e.Response == "" && route.Kind != KindSSE {
			t.Fatalf("SDK endpoint %q answers no row", e.ID)
		}
		if !e.SDK && e.Path == "" {
			t.Fatalf("endpoint %q has no path", e.ID)
		}
	}
}
