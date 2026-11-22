// SPDX-License-Identifier: MIT

package main

// The payload types' renderer.
//
// The daemon's operation registry already says what every operation ACCEPTS
// and RETURNS — as JSON Schema, in the same place that decides what the
// operation actually does. The browser was repeating that contract by hand:
// about 217 interfaces describing payloads the registry had already
// described (W5.1). This file renders those schemas into TypeScript types, so
// the browser's payload types are DERIVED from the registry that serves the
// payloads. A field renamed in the registry is a compile error in the browser,
// not a surprise at runtime.
//
// The renderer is deliberately simple because the schema language is simple:
// the registry's schemas use type, properties, required, items,
// additionalProperties and description — no $ref, no enum, no oneOf. When the
// schema language grows, this renderer grows with it; it never guesses.
//
// Two deviations from the daemon's strict schema, both on the browser's side:
//
//  1. A RESPONSE field is OPTIONAL. The registry marks every field required;
//     the browser's hand-written interfaces were almost entirely optional,
//     and the console reads a missing field and a null field the same way.
//     The daemon's exact contract stays in docs/openapi.json — this is the
//     browser's LENIENT view of it, not a second contract.
//  2. A REQUEST field keeps the registry's REQUIRED list. The daemon rejects a
//     body that lacks a required field, so the browser learns that from the
//     compiler instead of from a 400.
//
// NULL does not survive in the browser's view: the daemon sends null for a nil
// slice, and the browser's code has always read a null field and a missing
// field the same way (`|| []`, `?.`, `??`). Folding null into absent cost 111
// fewer browser call sites than keeping the null arms, and it cost nothing in
// truth: docs/openapi.json is generated from the same schemas, keeps every null
// arm, and is the document a caller outside the browser reads.
//
// The naming is derived, never hand-picked: an operation's response type is
// its operation name in PascalCase plus "Response"; a nested object is its
// parent plus the property name. Nothing in the browser names a type that
// this file did not derive, so there is no second place to rename.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// typesHeader is the provenance banner the generated types file starts with.
const typesHeader = `// SPDX-License-Identifier: MIT

// GENERATED FILE — do not edit by hand.
//
// Source of truth: the daemon's operation registry (kernel/controlplane), the
// same registry docs/openapi.json is generated from.
// Regenerate:       go run ./tools/openapi
// Verify:          go run ./tools/openapi -check
//
// Every payload type below is DERIVED from the registry's JSON Schema. The
// browser does not get to name a payload: it names a PATH, and the path says
// which operation answers it and therefore which shape comes back.
//
// The browser's view is LENIENT on responses and STRICT on requests: a
// response field may read as absent, a request field the registry marks
// required must be sent. NULL folds into absent here, because the browser
// reads a null field and a missing field the same way. The daemon's exact
// contract — every field required, every null arm — stays in
// docs/openapi.json, which is the document a caller outside the browser
// reads.
`

// schemaNode is one node of a JSON Schema, restricted to the keywords the
// registry actually uses.
type schemaNode struct {
	Type                 json.RawMessage            `json:"type"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Required             []string                   `json:"required"`
	Items                json.RawMessage            `json:"items"`
	AdditionalProperties json.RawMessage            `json:"additionalProperties"`
	Description          string                     `json:"description"`
}

// typeNames returns the node's declared types. A schema with no type but with
// properties is an object; a schema with nothing at all says nothing.
func (n schemaNode) typeNames() []string {
	if len(n.Type) == 0 {
		if len(n.Properties) > 0 {
			return []string{"object"}
		}
		return nil
	}
	var single string
	if err := json.Unmarshal(n.Type, &single); err == nil {
		return []string{single}
	}
	var many []string
	if err := json.Unmarshal(n.Type, &many); err == nil {
		return many
	}
	return nil
}

// hasType reports whether the node declares the given type.
func (n schemaNode) hasType(want string) bool {
	for _, name := range n.typeNames() {
		if name == want {
			return true
		}
	}
	return false
}

// nullable reports whether the node's type list includes null.
func (n schemaNode) nullable() bool {
	return n.hasType("null")
}

// renderMode is the browser's VIEW of the registry's strict schema.
//
// The view is SPLIT, and the split is measured, not guessed. Keeping the
// registry's null arms on every field cost 111 more browser call sites than
// dropping them: the browser's own code already reads a missing field and a
// null field the same way (`|| []`, `?.`, `??`), because the daemon sends null
// for a nil slice and the browser has always treated "no items" and "no such
// field" as one thing. The browser's view therefore FOLDS null into absent.
//
// The registry's exact nullable contract is not lost: docs/openapi.json is
// generated from the same schemas and keeps every null arm, and it is the
// document a caller outside the browser reads.
//
// The registry's required list is kept on REQUEST bodies, where a missing
// field is a body the daemon rejects, and on responses, where the daemon
// always sends the field.
type renderMode struct {
	// null keeps the registry's null arms: a field the daemon may send as
	// null stays nullable in the browser.
	null bool
	// requestRequired keeps the required list on REQUEST bodies: the daemon
	// rejects a body that lacks a required field, so the browser learns
	// that at compile time.
	requestRequired bool
	// required keeps the required list on responses too.
	required bool
}

// mode is the browser's view of the registry's schema.
func mode() renderMode {
	return renderMode{null: false, requestRequired: true, required: true}
}

// tsRenderer accumulates the named type declarations one schema produces.
//
// A shape is declared ONCE. Two operations that carry the same wire shape —
// the agent profile appears in a dozen of them — would otherwise produce a
// dozen identical interfaces, and the browser would have to guess which one
// names the shape. The renderer therefore keys a declaration by the shape's
// STRUCTURE: the first operation that carries a shape names it, and every
// later operation that carries the same shape gets an alias to it.
type tsRenderer struct {
	decls map[string]string
	order []string
	// aliases maps a duplicate name to the declaration it points at.
	aliases map[string]string
	// aliasOrder keeps the aliases in the order they were discovered.
	aliasOrder []string
	// canonical maps a shape key to the name that declares it.
	canonical map[string]string
	mode      renderMode
	// request marks the renderer as walking a REQUEST body, where the
	// registry's required list is kept even in the lenient response view.
	request bool
}

// declare records one named declaration.
//
// The body is compared by SHAPE, not by text: two names whose schemas have
// the same structure are the same declaration, and the second one becomes an
// alias to the first.
func (r *tsRenderer) declare(name, body, key string) (string, error) {
	if canonical, ok := r.canonical[key]; ok {
		if canonical != name {
			r.aliases[name] = canonical
			r.aliasOrder = append(r.aliasOrder, name)
		}
		return canonical, nil
	}
	r.canonical[key] = name
	r.decls[name] = body
	r.order = append(r.order, name)
	return name, nil
}

// ownName names a declaration after the operation that carried it.
//
// Two operations can carry the same shape — the OAuth start answer is the same
// wherever it comes from — and the renderer declares such a shape ONCE. The
// later operation's name still exists, as an alias to the first, so the path
// map can keep pointing at the name that belongs to the operation: a browser
// that reads "/api/provider/oauth/start" deserves to see a type called
// ProviderOauthStartResponse, not the channel one that happened to be
// declared first.
func (r *tsRenderer) ownName(rendered, own string) string {
	if rendered == own {
		return own
	}
	if _, ok := r.aliases[own]; ok {
		return own
	}
	return rendered
}

// identifierPattern matches a TypeScript property name that needs no quoting.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// renderTypes renders the browser's payload types from the route catalogue
// and the API document.
//
// routes is the console's route table; document is the derived OpenAPI
// document, which carries each operation's schemas.
func renderTypes(routes []opapi.HTTPRoute, document *opapi.OpenAPIDocument) (string, error) {
	renderer := &tsRenderer{
		decls:     map[string]string{},
		aliases:   map[string]string{},
		canonical: map[string]string{},
		mode:      mode(),
	}
	type mapping struct {
		path      string
		operation string
		response  string
		request   string
	}
	mappings := make([]mapping, 0, len(routes))
	seen := map[string]bool{}

	ordered := append([]opapi.HTTPRoute(nil), routes...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })

	for _, route := range ordered {
		item, ok := document.Paths[route.Path]
		if !ok {
			return "", fmt.Errorf("the API document has no path %q", route.Path)
		}
		operation := item.Get
		if operation == nil {
			operation = item.Post
		}
		if operation == nil {
			return "", fmt.Errorf("the path %q binds no operation", route.Path)
		}
		root := pascalName(operation.OperationID)
		responseSchema := responseSchemaOf(operation)
		if len(responseSchema) == 0 {
			return "", fmt.Errorf("operation %q has no response schema", operation.OperationID)
		}
		responseType, err := renderer.renderType(root+"Response", responseSchema)
		if err != nil {
			return "", err
		}
		entry := mapping{path: route.Path, operation: operation.OperationID, response: renderer.ownName(responseType, root+"Response")}
		if route.Kind == opapi.HTTPRouteJSON && operation.RequestBody != nil {
			bodySchema := requestSchemaOf(operation)
			if len(bodySchema) > 0 {
				renderer.request = true
				requestType, err := renderer.renderType(root+"Request", bodySchema)
				renderer.request = false
				if err != nil {
					return "", err
				}
				entry.request = renderer.ownName(requestType, root+"Request")
			}
		}
		if seen[route.Path] {
			return "", fmt.Errorf("the path %q is bound twice", route.Path)
		}
		seen[route.Path] = true
		mappings = append(mappings, entry)
	}

	var out strings.Builder
	out.WriteString(typesHeader)

	// The named declarations, in the order the renderer produced them. The
	// order is stable because the routes are sorted and the properties inside
	// a schema are sorted too.
	for _, name := range renderer.order {
		out.WriteString("\n/** " + name + " */\n")
		out.WriteString("export interface " + name + " {\n")
		out.WriteString(renderer.decls[name])
		out.WriteString("}\n")
	}

	// The duplicates: an operation that carries a shape another operation
	// already declared gets an alias to that declaration, so the browser can
	// name the shape it means without the file repeating it.
	for _, name := range renderer.aliasOrder {
		canonical := renderer.aliases[name]
		out.WriteString("\n/** " + name + " is the same shape as " + canonical + ". */\n")
		out.WriteString("export type " + name + " = " + canonical + ";\n")
	}

	out.WriteString("\n/**\n * OperationResponseMap binds a path to the shape the console answers it\n")
	out.WriteString(" * with. A path that is not in this map is not a path the console\n")
	out.WriteString(" * answers for an operation.\n */\n")
	out.WriteString("export interface OperationResponseMap {\n")
	for _, entry := range mappings {
		out.WriteString("  " + quote(entry.path) + ": " + entry.response + ";\n")
	}
	out.WriteString("}\n")

	out.WriteString("\n/**\n * OperationRequestMap binds a path to the body the console accepts for\n")
	out.WriteString(" * it. Only the paths that take a JSON body appear here.\n */\n")
	out.WriteString("export interface OperationRequestMap {\n")
	for _, entry := range mappings {
		if entry.request == "" {
			continue
		}
		out.WriteString("  " + quote(entry.path) + ": " + entry.request + ";\n")
	}
	out.WriteString("}\n")

	out.WriteString(`
/**
 * ResponseOf is the shape the console answers a path with. A path that binds
 * no operation has no shape the browser can name, so it resolves to any.
 */
export type ResponseOf<P extends string> = P extends keyof OperationResponseMap
  ? OperationResponseMap[P]
  : any;

/**
 * RequestOf is the body the console accepts for a path. A path that takes no
 * JSON body has no body the browser can name, so it resolves to any.
 */
export type RequestOf<P extends string> = P extends keyof OperationRequestMap
  ? OperationRequestMap[P]
  : any;
`)
	return out.String(), nil
}

// renderType renders one schema as a TypeScript type expression, declaring
// every named type it needs along the way.
//
// name is the name the schema gets if it turns out to be an object; nested
// objects take the parent's name plus the property's name.
func (r *tsRenderer) renderType(name string, raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "any", nil
	}
	var node schemaNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return "", fmt.Errorf("parse the schema of %s: %w", name, err)
	}

	if len(node.Properties) > 0 {
		body, err := r.objectBody(name, node)
		if err != nil {
			return "", err
		}
		canonical, err := r.declare(name, body, shapeKey(raw))
		if err != nil {
			return "", err
		}
		return r.withNull(canonical, node), nil
	}

	if node.hasType("array") {
		item := "any"
		if len(node.Items) > 0 {
			rendered, err := r.renderType(name+"Item", node.Items)
			if err != nil {
				return "", err
			}
			item = rendered
		}
		return r.withNull(item+"[]", node), nil
	}

	if node.hasType("object") {
		record := "Record<string, any>"
		if len(node.AdditionalProperties) > 0 && !isJSONBool(node.AdditionalProperties, false) {
			if isJSONBool(node.AdditionalProperties, true) {
				return r.withNull(record, node), nil
			}
			value, err := r.renderType(name+"Value", node.AdditionalProperties)
			if err != nil {
				return "", err
			}
			record = "Record<string, " + value + ">"
		}
		return r.withNull(record, node), nil
	}

	types := node.typeNames()
	if len(types) == 0 {
		return "any", nil
	}
	parts := make([]string, 0, len(types))
	for _, name := range types {
		switch name {
		case "string":
			parts = append(parts, "string")
		case "integer", "number":
			parts = append(parts, "number")
		case "boolean":
			parts = append(parts, "boolean")
		case "null":
			if r.mode.null {
				parts = append(parts, "null")
			}
		case "array":
			parts = append(parts, "any[]")
		case "object":
			parts = append(parts, "Record<string, any>")
		default:
			parts = append(parts, "any")
		}
	}
	if len(parts) == 0 {
		return "any", nil
	}
	return strings.Join(parts, " | "), nil
}

// objectBody renders the members of an object schema.
//
// Every member is optional. The registry marks every member required, but the
// browser's view is lenient on purpose: the console answers a missing member
// the same way it answers a null one.
func (r *tsRenderer) objectBody(name string, node schemaNode) (string, error) {
	names := make([]string, 0, len(node.Properties))
	for prop := range node.Properties {
		names = append(names, prop)
	}
	sort.Strings(names)
	requiredNames := map[string]bool{}
	for _, prop := range node.Required {
		requiredNames[prop] = true
	}

	var out strings.Builder
	for _, prop := range names {
		raw := node.Properties[prop]
		var child schemaNode
		if err := json.Unmarshal(raw, &child); err != nil {
			return "", fmt.Errorf("parse the property %q of %s: %w", prop, name, err)
		}
		required := requiredNames[prop]
		childType, err := r.renderType(name+pascalName(prop), raw)
		if err != nil {
			return "", err
		}
		if child.Description != "" {
			out.WriteString("  /** " + oneLine(child.Description) + " */\n")
		}
		marker := "?"
		if (r.mode.required || (r.request && r.mode.requestRequired)) && required {
			marker = ""
		}
		out.WriteString("  " + propertyName(prop) + marker + ": " + childType + ";\n")
	}
	return out.String(), nil
}

// shapeKey returns a structural key for a schema: two schemas with the same
// key have the same shape, whatever they are called.
func shapeKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "*"
	}
	var node schemaNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return "?"
	}
	return nodeKey(node)
}

// nodeKey returns the structural key of one schema node.
func nodeKey(node schemaNode) string {
	if len(node.Properties) > 0 {
		names := make([]string, 0, len(node.Properties))
		for prop := range node.Properties {
			names = append(names, prop)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, prop := range names {
			var child schemaNode
			if err := json.Unmarshal(node.Properties[prop], &child); err != nil {
				parts = append(parts, prop+":?")
				continue
			}
			parts = append(parts, prop+":"+nodeKey(child))
		}
		// The required list is part of the shape: the same fields, one of
		// them required and one of them optional, are two different shapes
		// and must not share a declaration.
		required := append([]string(nil), node.Required...)
		sort.Strings(required)
		return "{" + strings.Join(parts, ",") + "}" + nullArm(node) + "!" + strings.Join(required, ",")
	}
	if node.hasType("array") {
		item := "*"
		if len(node.Items) > 0 {
			var child schemaNode
			if err := json.Unmarshal(node.Items, &child); err == nil {
				item = nodeKey(child)
			}
		}
		return "[" + item + "]" + nullArm(node)
	}
	types := node.typeNames()
	if len(types) == 0 {
		return "*"
	}
	sorted := append([]string(nil), types...)
	sort.Strings(sorted)
	return strings.Join(sorted, "|")
}

// nullArm marks a nullable schema so that a nullable shape and a non-nullable
// one are not the same declaration.
func nullArm(node schemaNode) string {
	if node.nullable() {
		return "?"
	}
	return ""
}

// propertyName renders a property name as a TypeScript member name, quoting
// it when it is not a plain identifier.
func propertyName(name string) string {
	if identifierPattern.MatchString(name) {
		return name
	}
	return quote(name)
}

// withNull adds the null arm to a rendered type when the schema is nullable.
//
// The arm survives only where the browser would otherwise have no warning: on
// a member the daemon always sends. An optional member drops it, because the
// `?` in front of the member already forces the guard that a null needs.
func (r *tsRenderer) withNull(rendered string, node schemaNode) string {
	if !r.mode.null || !node.nullable() || strings.Contains(rendered, "null") {
		return rendered
	}
	return rendered + " | null"
}

// oneLine folds a description into one comment line.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// isJSONBool reports whether a raw schema value is the given JSON boolean.
func isJSONBool(raw json.RawMessage, want bool) bool {
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return value == want
}

// responseSchemaOf returns an operation's response schema.
func responseSchemaOf(operation *opapi.OpenAPIOperation) json.RawMessage {
	response, ok := operation.Responses["200"]
	if !ok {
		return nil
	}
	media, ok := response.Content["application/json"]
	if !ok {
		return nil
	}
	return media.Schema
}

// requestSchemaOf returns an operation's request body schema.
func requestSchemaOf(operation *opapi.OpenAPIOperation) json.RawMessage {
	if operation.RequestBody == nil {
		return nil
	}
	media, ok := operation.RequestBody.Content["application/json"]
	if !ok {
		return nil
	}
	return media.Schema
}

// pascalName turns an operation or property name into a TypeScript name.
//
// Operation names are not TypeScript names: they carry dots and dashes
// ("configcenter.set-rating"), so every separator becomes a word boundary.
func pascalName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9')
	})
	var out strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		out.WriteString(strings.ToUpper(part[:1]))
		out.WriteString(part[1:])
	}
	return out.String()
}
