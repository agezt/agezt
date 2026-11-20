// SPDX-License-Identifier: MIT

package opapi

// The OpenAPI producer's own tests.
//
// The document is DERIVED: it can only say what the catalogue says and what
// the registry says. These tests pin that join — every catalogue path is in
// the document, every operation carries the registry's schema, and the route
// kind decides the HTTP shape (a query route has parameters, a body route has
// a request body).

import (
	"encoding/json"
	"strings"
	"testing"
)

// everyOpSchema is a schema source that answers for every operation with a
// minimal, valid contract.
func everyOpSchema(op string) (OperationSchema, bool) {
	return OperationSchema{
		Input:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
		Output: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
	}, true
}

// buildTestDocument builds the document from the real catalogue.
func buildTestDocument(t *testing.T) *OpenAPIDocument {
	t.Helper()
	doc, err := BuildOpenAPI(everyOpSchema, OpenAPIInfo{Title: "test", Version: "0"})
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	return doc
}

// TestTheDocumentDescribesTheWholeCatalogue: every catalogue path is in the
// document, with the catalogue's method and operation name. A path in the
// catalogue that the document does not describe is a route the console serves
// and the API says nothing about.
func TestTheDocumentDescribesTheWholeCatalogue(t *testing.T) {
	doc := buildTestDocument(t)
	routes := ConsoleRoutes()
	if len(doc.Paths) != len(routes) {
		t.Fatalf("the document describes %d paths, the catalogue has %d", len(doc.Paths), len(routes))
	}
	for _, route := range routes {
		item, ok := doc.Paths[route.Path]
		if !ok {
			t.Errorf("%s is in the catalogue but not in the document", route.Path)
			continue
		}
		var operation *OpenAPIOperation
		switch route.Method {
		case "GET":
			operation = item.Get
		case "POST":
			operation = item.Post
		}
		if operation == nil {
			t.Errorf("%s answers %s, but the document has no such operation", route.Path, route.Method)
			continue
		}
		if operation.OperationID != route.Op {
			t.Errorf("%s is documented as operation %q, the catalogue says %q", route.Path, operation.OperationID, route.Op)
		}
	}
}

// TestTheRouteKindDecidesTheHTTPShape: a parameterless read takes nothing, a
// query route publishes its parameters, and a body route publishes a body.
// Getting this wrong is how a browser's payload goes nowhere.
func TestTheRouteKindDecidesTheHTTPShape(t *testing.T) {
	doc := buildTestDocument(t)
	byPath := map[string]HTTPRoute{}
	for _, route := range ConsoleRoutes() {
		byPath[route.Path] = route
	}
	for _, route := range ConsoleRoutes() {
		item := doc.Paths[route.Path]
		operation := item.Get
		if operation == nil {
			operation = item.Post
		}
		if operation == nil {
			t.Fatalf("%s has no operation", route.Path)
		}
		switch route.Kind {
		case HTTPRouteRead:
			if len(operation.Parameters) != 0 || operation.RequestBody != nil {
				t.Errorf("%s is a parameterless read but the document publishes parameters or a body", route.Path)
			}
		case HTTPRouteReadArgs, HTTPRouteWrite:
			if len(operation.Parameters) != len(route.Args) {
				t.Errorf("%s forwards %v but the document publishes %d parameters",
					route.Path, route.Args, len(operation.Parameters))
			}
			for i, parameter := range operation.Parameters {
				if parameter.Name != route.Args[i] || parameter.In != "query" {
					t.Errorf("%s parameter %d is %q in %q, want %q in query",
						route.Path, i, parameter.Name, parameter.In, route.Args[i])
				}
			}
			if operation.RequestBody != nil {
				t.Errorf("%s is a query route but the document publishes a request body", route.Path)
			}
		case HTTPRouteJSON, HTTPRouteStream:
			if operation.RequestBody == nil {
				t.Errorf("%s is a body route but the document publishes no request body", route.Path)
			}
			if len(operation.Parameters) != 0 {
				t.Errorf("%s is a body route but the document publishes query parameters", route.Path)
			}
		}
	}
}

// TestTheQueryRoutePublishesTheConsolesAllowlist: the document names the
// arguments the CONSOLE forwards, which is the console's own allowlist — not
// the operation's wider input schema. The browser may only set these.
func TestTheQueryRoutePublishesTheConsolesAllowlist(t *testing.T) {
	doc := buildTestDocument(t)
	for _, route := range ConsoleRoutes() {
		if route.Kind != HTTPRouteReadArgs && route.Kind != HTTPRouteWrite {
			continue
		}
		item := doc.Paths[route.Path]
		operation := item.Get
		if operation == nil {
			operation = item.Post
		}
		names := map[string]bool{}
		for _, parameter := range operation.Parameters {
			names[parameter.Name] = true
		}
		for _, arg := range route.Args {
			if !names[arg] {
				t.Errorf("%s forwards %q but the document does not publish it", route.Path, arg)
			}
		}
	}
}

// TestTheResponseCarriesTheRegistrysOutputSchema: the document describes what
// the operation RETURNS, from the registry — never a hand-written copy.
func TestTheResponseCarriesTheRegistrysOutputSchema(t *testing.T) {
	doc := buildTestDocument(t)
	for path, item := range doc.Paths {
		operation := item.Get
		if operation == nil {
			operation = item.Post
		}
		response, ok := operation.Responses["200"]
		if !ok {
			t.Errorf("%s has no 200 response", path)
			continue
		}
		media, ok := response.Content["application/json"]
		if !ok {
			t.Errorf("%s has no JSON response content", path)
			continue
		}
		if !strings.Contains(string(media.Schema), `"ok"`) {
			t.Errorf("%s publishes a response schema that is not the registry's", path)
		}
	}
}

// TestAnOperationWithoutAContractStopsTheDocument: a registry that cannot
// answer for a catalogue route is a missing contract. The document says so
// rather than publishing a route with no shape.
func TestAnOperationWithoutAContractStopsTheDocument(t *testing.T) {
	routes := ConsoleRoutes()
	if len(routes) == 0 {
		t.Fatal("the catalogue is empty")
	}
	missing := routes[0].Op
	_, err := BuildOpenAPI(func(op string) (OperationSchema, bool) {
		if op == missing {
			return OperationSchema{}, false
		}
		return everyOpSchema(op)
	}, OpenAPIInfo{Title: "test", Version: "0"})
	if err == nil {
		t.Fatal("the document was built although one operation had no contract")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error names %q, want the operation without a contract (%q)", err.Error(), missing)
	}
}

// TestTheDocumentNeedsASchemaSource: without a registry there is nothing to
// derive the shapes from.
func TestTheDocumentNeedsASchemaSource(t *testing.T) {
	if _, err := BuildOpenAPI(nil, OpenAPIInfo{}); err == nil {
		t.Fatal("the document was built without a schema source")
	}
}

// TestTheMarshalledDocumentIsStable: the file on disk must be diffable, so
// the rendering is indented JSON with a trailing newline.
func TestTheMarshalledDocumentIsStable(t *testing.T) {
	raw, err := MarshalOpenAPI(buildTestDocument(t))
	if err != nil {
		t.Fatalf("MarshalOpenAPI: %v", err)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Error("the rendered document does not end with a newline")
	}
	if !json.Valid(raw) {
		t.Error("the rendered document is not valid JSON")
	}
	if !strings.Contains(string(raw), "\n  \"paths\"") {
		t.Error("the rendered document is not indented")
	}
}

// TestTheOperationIDsAreSortedAndComplete: the document's operation list is
// the catalogue's, sorted — a stable order is what a diff needs.
func TestTheOperationIDsAreSortedAndComplete(t *testing.T) {
	doc := buildTestDocument(t)
	ids := OpenAPIOperationIDs(doc)
	if len(ids) != len(doc.Paths) {
		t.Fatalf("the document lists %d operations for %d paths", len(ids), len(doc.Paths))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Fatalf("the operation list is not sorted: %q before %q", ids[i-1], ids[i])
		}
	}
	for _, route := range ConsoleRoutes() {
		found := false
		for _, id := range ids {
			if id == route.Op {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the document does not list operation %q", route.Op)
		}
	}
}
