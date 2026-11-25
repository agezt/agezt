// SPDX-License-Identifier: MIT

package restapiapi

import (
	"encoding/json"
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

// TestEverySDKEndpointExchangesPublishedRows holds the SDK catalog's row set
// to the SDK contract: a row an SDK endpoint exchanges is published to the
// SDKs, or the catalog would miss a shape the SDKs speak.
func TestEverySDKEndpointExchangesPublishedRows(t *testing.T) {
	for _, e := range Endpoints() {
		if !e.SDK {
			continue
		}
		for _, name := range []string{e.Request, e.Response} {
			if name == "" {
				continue
			}
			if !rowByName(t, name).SDK {
				t.Errorf("the SDK endpoint %q exchanges the %q row, which the SDK catalog does not carry",
					e.ID, name)
			}
		}
	}
}

// TestTheSelfUpdateBodyStaysOutOfTheSDKCatalog holds the self-update body's
// place: the SDKs do not expose that surface, so its request row stays in the
// contract and out of the SDKs' public API — and the daemon's door still has
// a row to measure the body against.
func TestTheSelfUpdateBodyStaysOutOfTheSDKCatalog(t *testing.T) {
	if rowByName(t, RowUpdateApply).SDK {
		t.Errorf("the %q row is published to the SDKs — the SDKs do not expose the self-update surface",
			RowUpdateApply)
	}
	for _, r := range SDKRows() {
		if r.Name == RowUpdateApply {
			t.Errorf("the SDK catalog carries the %q row", RowUpdateApply)
		}
	}
	for _, r := range SDKRequestRows() {
		if r.Name == RowUpdateApply {
			t.Errorf("the SDK request rows hold the %q row — no SDK builds that body", RowUpdateApply)
		}
	}
	found := false
	for _, r := range RequestRows() {
		if r.Name == RowUpdateApply {
			found = true
		}
	}
	if !found {
		t.Errorf("the surface's request rows do not hold the %q row — the door has no schema to measure against",
			RowUpdateApply)
	}
	if _, ok := RequestSchema(EndpointUpdateApply); !ok {
		t.Errorf("the %q endpoint has no request schema", EndpointUpdateApply)
	}
}

// TestTheRouteAndTheEndpointAgreeOnTheRequestRow: the daemon registers the
// route and the SDKs call the endpoint, so both must name the same body —
// otherwise the door would measure a body the route does not take.
func TestTheRouteAndTheEndpointAgreeOnTheRequestRow(t *testing.T) {
	for _, e := range Endpoints() {
		route := routeByID(t, e.Route)
		for _, m := range route.Methods {
			if m.Verb != e.Verb {
				continue
			}
			if m.Request != e.Request {
				t.Errorf("the %q endpoint takes the %q request row, the %q route's %s method takes %q",
					e.ID, e.Request, route.ID, m.Verb, m.Request)
			}
		}
	}
}

// TestEveryRowRendersASchemaTheDoorCanCheck holds the shape the daemon's
// request door measures a body against.
//
// Three rules, and each one is a rule the door has:
//   - every field renders a TYPE the validator speaks — a wire type the
//     mapping does not know would render an empty type, and the door would
//     then accept anything for that field;
//   - the schema carries no `required` list — the registry's door checks
//     TYPES, and presence is the handler's business;
//   - an undeclared field stays allowed — a row is the stable SUBSET of a
//     body, so the daemon may read more than the contract names.
//
// It walks every row of the vocabulary, so a row added later is covered
// without this test knowing its name.
func TestEveryRowRendersASchemaTheDoorCanCheck(t *testing.T) {
	for _, r := range Rows() {
		raw, ok := rowSchema(r)
		if !ok {
			t.Fatalf("the %q row renders no schema", r.Name)
		}
		var doc struct {
			Type                 string                     `json:"type"`
			AdditionalProperties bool                       `json:"additionalProperties"`
			Required             []string                   `json:"required"`
			Properties           map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("the %q row renders an unreadable schema: %v", r.Name, err)
		}
		if doc.Type != "object" {
			t.Fatalf("the %q row renders the %q schema, want object", r.Name, doc.Type)
		}
		if len(doc.Required) != 0 {
			t.Fatalf("the %q row's schema carries the required list %v — the door checks types, not presence",
				r.Name, doc.Required)
		}
		if !doc.AdditionalProperties {
			t.Fatalf("the %q row's schema does not allow an undeclared field — a row is the stable subset of a body",
				r.Name)
		}
		if len(doc.Properties) != len(r.Fields) {
			t.Fatalf("the %q row renders %d properties, want %d", r.Name, len(doc.Properties), len(r.Fields))
		}
		for name, raw := range doc.Properties {
			var prop struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &prop); err != nil {
				t.Fatalf("the %q row's %q renders an unreadable property: %v", r.Name, name, err)
			}
			if prop.Type == "" {
				t.Fatalf("the %q row's %q renders no type — the door would accept anything for it",
					r.Name, name)
			}
		}
	}
}
