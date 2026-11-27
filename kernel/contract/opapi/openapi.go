// SPDX-License-Identifier: MIT

package opapi

// OpenAPI 3.1 document generation.
//
// The daemon's operation registry and the console's route catalogue are the
// two halves of the console's HTTP contract: the catalogue says WHICH path
// answers WHICH operation and which arguments the console forwards; the
// registry says what each operation ACCEPTS and RETURNS. An OpenAPI document
// is the join of the two — it is derived, never written by hand.
//
// This file lives next to the catalogue for the same reason the catalogue
// lives here (W3.82): a contract that describes the console's HTTP surface
// belongs with the operation names it binds, and a second hand-written copy
// of the surface is a copy that drifts.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// OpenAPI version this package emits.
const openAPIVersion = "3.1.0"

// OperationSchema is one operation's wire contract: the JSON Schema of its
// input and of its output. Both are empty when the operation declares none.
type OperationSchema struct {
	Input  json.RawMessage
	Output json.RawMessage
}

// OpenAPISchemaSource resolves an operation's wire contract by operation name.
// A registry that cannot answer for an operation is a missing contract, and
// the document says so rather than publishing a route with no shape.
type OpenAPISchemaSource func(op string) (OperationSchema, bool)

// OpenAPIInfo is the document's metadata block.
type OpenAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

// OpenAPIPathItem holds the operations one path answers. A path is bound to
// exactly one operation, so at most one method is present.
type OpenAPIPathItem struct {
	Get  *OpenAPIOperation `json:"get,omitempty"`
	Post *OpenAPIOperation `json:"post,omitempty"`
}

// OpenAPIOperation is one operation's OpenAPI representation.
type OpenAPIOperation struct {
	OperationID string                     `json:"operationId"`
	Summary     string                     `json:"summary,omitempty"`
	Tags        []string                   `json:"tags,omitempty"`
	Parameters  []OpenAPIParameter         `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
}

// OpenAPIParameter is one query parameter the console forwards.
type OpenAPIParameter struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// OpenAPIRequestBody describes a JSON request body.
type OpenAPIRequestBody struct {
	Required bool                        `json:"required,omitempty"`
	Content  map[string]OpenAPIMediaType `json:"content"`
}

// OpenAPIMediaType is one media type of a request body or response.
type OpenAPIMediaType struct {
	Schema json.RawMessage `json:"schema,omitempty"`
}

// OpenAPIResponse is one response of an operation.
type OpenAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]OpenAPIMediaType `json:"content,omitempty"`
}

// OpenAPIDocument is the OpenAPI 3.1 document.
type OpenAPIDocument struct {
	OpenAPI string                     `json:"openapi"`
	Info    OpenAPIInfo                `json:"info"`
	Paths   map[string]OpenAPIPathItem `json:"paths"`
}

// BuildOpenAPI derives the OpenAPI document from the console's route
// catalogue and the daemon's operation registry.
//
// The document is DERIVED, so it cannot disagree with the two sources: every
// path in it is a catalogue path, and every operation in it carries the
// registry's own input and output schema.
func BuildOpenAPI(schemas OpenAPISchemaSource, info OpenAPIInfo) (*OpenAPIDocument, error) {
	if schemas == nil {
		return nil, fmt.Errorf("opapi: an operation schema source is required")
	}
	routes := ConsoleRoutes()
	if len(routes) == 0 {
		return nil, fmt.Errorf("opapi: the console route catalogue is empty")
	}
	doc := &OpenAPIDocument{
		OpenAPI: openAPIVersion,
		Info:    info,
		Paths:   make(map[string]OpenAPIPathItem, len(routes)),
	}
	for _, route := range routes {
		schema, ok := schemas(route.Op)
		if !ok {
			return nil, fmt.Errorf("opapi: no wire contract for operation %q (%s)", route.Op, route.Path)
		}
		operation, err := openAPIOperation(route, schema)
		if err != nil {
			return nil, err
		}
		item := OpenAPIPathItem{}
		switch route.Method {
		case "GET":
			item.Get = operation
		case "POST":
			item.Post = operation
		default:
			return nil, fmt.Errorf("opapi: operation %q uses unsupported method %q", route.Op, route.Method)
		}
		doc.Paths[route.Path] = item
	}
	return doc, nil
}

// openAPIOperation renders one catalogue route as an OpenAPI operation.
func openAPIOperation(route HTTPRoute, schema OperationSchema) (*OpenAPIOperation, error) {
	operation := &OpenAPIOperation{
		OperationID: route.Op,
		Summary:     routeSummary(route),
		Tags:        []string{routeTag(route.Op)},
		Responses:   map[string]OpenAPIResponse{},
	}
	switch route.Kind {
	case HTTPRouteRead:
		// A parameterless read takes nothing.
	case HTTPRouteReadArgs:
		operation.Parameters = queryParameters(route.Args)
	case HTTPRouteWrite:
		operation.Parameters = queryParameters(route.Args)
	case HTTPRouteJSON:
		body, err := requestBody(route, schema.Input)
		if err != nil {
			return nil, err
		}
		operation.RequestBody = body
	case HTTPRouteStream:
		body, err := requestBody(route, schema.Input)
		if err != nil {
			return nil, err
		}
		operation.RequestBody = body
	default:
		return nil, fmt.Errorf("opapi: operation %q has unknown route kind %d", route.Op, route.Kind)
	}
	response := OpenAPIResponse{Description: "OK"}
	if len(schema.Output) > 0 {
		if !json.Valid(schema.Output) {
			return nil, fmt.Errorf("opapi: operation %q has an invalid output schema", route.Op)
		}
		response.Content = map[string]OpenAPIMediaType{
			"application/json": {Schema: schema.Output},
		}
	}
	operation.Responses["200"] = response
	return operation, nil
}

// queryParameters renders a route's forwarded argument allowlist as OpenAPI
// query parameters. The allowlist is the console's OWN list, so the document
// publishes exactly the arguments the console forwards — not the operation's
// full input schema, which is wider.
func queryParameters(args []string) []OpenAPIParameter {
	if len(args) == 0 {
		return nil
	}
	parameters := make([]OpenAPIParameter, 0, len(args))
	for _, arg := range args {
		parameters = append(parameters, OpenAPIParameter{
			Name:   arg,
			In:     "query",
			Schema: json.RawMessage(`{"type":"string"}`),
		})
	}
	return parameters
}

// requestBody renders an operation's JSON body. A route that forwards no
// argument has no body.
func requestBody(route HTTPRoute, input json.RawMessage) (*OpenAPIRequestBody, error) {
	if len(input) == 0 {
		return nil, nil
	}
	if !json.Valid(input) {
		return nil, fmt.Errorf("opapi: operation %q has an invalid input schema", route.Op)
	}
	return &OpenAPIRequestBody{
		Required: schemaRequiresFields(input),
		Content: map[string]OpenAPIMediaType{
			"application/json": {Schema: input},
		},
	}, nil
}

// schemaRequiresFields reports whether a JSON Schema declares at least one
// required property. A body whose fields are all optional is not required;
// one that names a required field is.
func schemaRequiresFields(schema json.RawMessage) bool {
	var decoded struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return false
	}
	return len(decoded.Required) > 0
}

// routeSummary writes the operation's one-line description: what it answers
// at, what it forwards, and — when the console keeps an argument for itself —
// what it consumes.
func routeSummary(route HTTPRoute) string {
	summary := route.Op + " — " + route.Method + " " + route.Path
	if len(route.Args) > 0 {
		summary += " (forwards " + strings.Join(route.Args, ", ") + ")"
	}
	if len(route.Consumed) > 0 {
		summary += " (consumes " + strings.Join(route.Consumed, ", ") + ")"
	}
	return summary
}

// routeTag groups an operation by the domain its name belongs to: the part
// before the first underscore.
func routeTag(op string) string {
	if index := strings.Index(op, "_"); index > 0 {
		return op[:index]
	}
	return op
}

// MarshalOpenAPI renders the document as indented JSON with a trailing
// newline, so a committed file is stable and diffable.
func MarshalOpenAPI(doc *OpenAPIDocument) ([]byte, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// OpenAPIOperationIDs returns every operation the document declares, sorted.
// The catalogue is the source; the document cannot invent one.
func OpenAPIOperationIDs(doc *OpenAPIDocument) []string {
	ids := make([]string, 0, len(doc.Paths))
	for _, item := range doc.Paths {
		switch {
		case item.Get != nil:
			ids = append(ids, item.Get.OperationID)
		case item.Post != nil:
			ids = append(ids, item.Post.OperationID)
		}
	}
	sort.Strings(ids)
	return ids
}
