// SPDX-License-Identifier: MIT

package openaiapi

// The OpenAI-compatible surface's request door.
//
// The contract names what a request body looks like (kernel/contract/openaiapi)
// and the door checks an arriving body against that shape BEFORE the handler
// reads it. The refusal carries the operation registry's own words:
//
//	operation <route> input: $.<field> has wrong type: got <got>, want <want>
//
// The tests below hold the door to four things:
//
//  1. the contract's schema and the daemon's own struct name the SAME fields
//     with the SAME types — the vocabulary has one home;
//  2. a mistyped field — and a null — is refused with the registry's words,
//     for every field of every body;
//  3. a body that fits the contract gets through untouched;
//  4. the surface's own refusals (a message list with no usable content) are
//     still the surface's own.
//
// The expectation is derived from the DAEMON'S OWN STRUCT, not from the
// contract's schema: a schema that mistyped a field would then agree with
// itself and pass.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	oaiapi "github.com/agezt/agezt/kernel/contract/openaiapi"
)

// rawMessageType is the Go type a two-shaped field is read as.
var rawMessageType = reflect.TypeOf(json.RawMessage{})

// doorBody is one route that takes a JSON body: where the door is reachable,
// and which struct the daemon reads the body into.
type doorBody struct {
	ID   string
	Path string
	Body reflect.Type
}

// doorBodies are the surface's JSON request bodies.
//
// The list is derived from the CONTRACT — a route the contract gives a body
// is a route the door must guard — and each entry names the daemon's own
// struct, so the test states its expectations from the daemon's side.
func doorBodies(t *testing.T) []doorBody {
	t.Helper()
	structs := map[string]reflect.Type{
		oaiapi.RouteChatCompletions: reflect.TypeOf(chatRequest{}),
		oaiapi.RouteResponses:       reflect.TypeOf(responsesRequest{}),
	}
	var out []doorBody
	for _, r := range oaiapi.Routes() {
		if len(r.Body) == 0 {
			continue
		}
		typ, ok := structs[r.ID]
		if !ok {
			t.Errorf("the %q route takes a body, but the daemon's struct for it is not in doorBodies() — "+
				"the door cannot be tested for that route", r.ID)
			continue
		}
		out = append(out, doorBody{ID: r.ID, Path: r.Path, Body: typ})
	}
	if len(out) == 0 {
		t.Fatal("no route takes a JSON request body — the door has nothing to guard")
	}
	return out
}

// doorField is one field the daemon reads: where it sits in the body, the Go
// type it is read as, and the registry's words for that type.
type doorField struct {
	Path  string       // "model", "messages[].role"
	Type  reflect.Type // the Go type the field is read as
	Words []string     // the registry's words for the type ("string")
}

// doorDirectFields lists a struct's OWN wire fields.
func doorDirectFields(t reflect.Type) []doorField {
	t = deref(t)
	var out []doorField
	if t == nil || t == rawMessageType || t.Kind() != reflect.Struct {
		return out
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, doorField{Path: name, Type: f.Type, Words: doorWords(f.Type)})
	}
	return out
}

// doorFields lists every wire field the daemon reads, the nested ones too.
func doorFields(t reflect.Type, prefix string) []doorField {
	var out []doorField
	for _, f := range doorDirectFields(t) {
		path := f.Path
		if prefix != "" {
			path = prefix + "." + f.Path
		}
		out = append(out, doorField{Path: path, Type: f.Type, Words: f.Words})
		out = append(out, doorFields(f.Type, path)...)
		if elem := deref(f.Type); elem.Kind() == reflect.Slice {
			out = append(out, doorFields(elem.Elem(), path+"[]")...)
		}
	}
	return out
}

// doorWords are the registry's words for a Go type — the type the daemon
// reads a field as.
//
// A json.RawMessage is the one field this surface reads in two shapes: the
// text itself, or an array of typed parts (a chat message's content, the
// Responses input). It is the only union here, and the registry's own words
// name it.
func doorWords(t reflect.Type) []string {
	if t == rawMessageType {
		return []string{"string", "array"}
	}
	switch deref(t).Kind() {
	case reflect.String:
		return []string{"string"}
	case reflect.Bool:
		return []string{"boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []string{"number"}
	case reflect.Slice, reflect.Array:
		return []string{"array"}
	case reflect.Map:
		return []string{"object"}
	case reflect.Struct:
		return []string{"object"}
	}
	return nil
}

// deref follows the pointers a handler's struct uses to say "this field may
// be absent".
func deref(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// doorValue is a value that fits a Go type, and a value that does NOT fit it.
func doorValue(t reflect.Type) (valid any, wrong any) {
	t = deref(t)
	if t == rawMessageType {
		return "x", 42
	}
	switch t.Kind() {
	case reflect.String:
		return "x", 42
	case reflect.Bool:
		return false, "yes"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return 7, "7"
	case reflect.Slice, reflect.Array:
		elem := t.Elem()
		v, _ := doorValue(elem)
		return []any{v}, "x"
	case reflect.Map:
		return map[string]any{"k": "v"}, "x"
	case reflect.Struct:
		out := map[string]any{}
		for _, f := range doorDirectFields(t) {
			v, _ := doorValue(f.Type)
			out[f.Path] = v
		}
		return out, "x"
	}
	return nil, nil
}

// doorBodyFor builds a body that fits the struct, except for the one field
// the caller mistypes (mistyped != "").
func doorBodyFor(t *testing.T, typ reflect.Type, mistyped string, value any) string {
	t.Helper()
	body := map[string]any{}
	known := false
	for _, f := range doorDirectFields(typ) {
		if f.Path == mistyped {
			known = true
			body[f.Path] = value
			continue
		}
		body[f.Path] = doorBuildValue(f.Type, f.Path, mistyped, value)
	}
	if mistyped != "" && !known {
		// A field the daemon does not read: the contract may still allow it.
		body[mistyped] = value
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal the body: %v", err)
	}
	return string(raw)
}

// doorBuildValue is the value a field carries in a body that fits — unless
// this field is the one the caller mistypes.
func doorBuildValue(typ reflect.Type, path, mistyped string, value any) any {
	if path == mistyped {
		return value
	}
	typ = deref(typ)
	if typ == rawMessageType {
		return "x"
	}
	switch typ.Kind() {
	case reflect.String:
		return "x"
	case reflect.Bool:
		return false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return 7
	case reflect.Slice, reflect.Array:
		return []any{doorBuildValue(typ.Elem(), path+"[]", mistyped, value)}
	case reflect.Map:
		return map[string]any{"k": "v"}
	case reflect.Struct:
		out := map[string]any{}
		for _, f := range doorDirectFields(typ) {
			out[f.Path] = doorBuildValue(f.Type, path+"."+f.Path, mistyped, value)
		}
		return out
	}
	return nil
}

// doorJSONPath turns a field path into the path the registry's validator
// names: "messages[].role" → "$.messages[0].role".
func doorJSONPath(path string) string {
	return "$." + strings.ReplaceAll(path, "[]", "[0]")
}

// doorPost posts a body to the surface and answers the status and the body.
func doorPost(t *testing.T, handler http.Handler, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// doorErrorMessage reads the `error.message` the surface answered with.
func doorErrorMessage(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode the answer %s: %v", body, err)
	}
	return payload.Error.Message
}

// doorSchema is a node of a JSON Schema, as the door's validator reads it.
type doorSchema struct {
	Type       any                   `json:"type"`
	Properties map[string]doorSchema `json:"properties"`
	Items      *doorSchema           `json:"items"`
}

// doorSchemaWords are the type words a schema node declares.
func doorSchemaWords(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	return nil
}

// TestTheContractAndTheDaemonNameTheSameBody is the ratchet: the contract's
// schema and the daemon's own struct name the same fields, with the same
// types, in both directions.
//
// Without the type half, a schema that mistyped a field agreed with itself
// and passed — the door would then measure a body against a type the handler
// cannot hold.
func TestTheContractAndTheDaemonNameTheSameBody(t *testing.T) {
	for _, route := range doorBodies(t) {
		shape, ok := oaiapi.RequestSchema(route.ID)
		if !ok {
			t.Errorf("the %q route has no schema — the door cannot measure its body", route.ID)
			continue
		}
		var doc doorSchema
		if err := json.Unmarshal(shape, &doc); err != nil {
			t.Fatalf("the %q route's schema is not readable: %v", route.ID, err)
		}
		doorCompareSchema(t, route.ID, "", route.Body, doc)
	}
}

// doorCompareSchema holds one node of the contract's schema to the daemon's
// own type: the same words, and the same fields, in both directions.
func doorCompareSchema(t *testing.T, routeID, path string, typ reflect.Type, node doorSchema) {
	t.Helper()
	want := doorWords(typ)
	got := doorSchemaWords(node.Type)
	if strings.Join(want, "|") != strings.Join(got, "|") {
		t.Errorf("the contract's %q schema names %q as %v, the daemon reads it as %v",
			routeID, path, got, want)
	}
	typ = deref(typ)
	if typ == nil || typ == rawMessageType {
		return
	}
	switch typ.Kind() {
	case reflect.Struct:
		for _, f := range doorDirectFields(typ) {
			child, ok := node.Properties[f.Path]
			if !ok {
				t.Errorf("the contract's %q schema does not name %q, which the daemon reads",
					routeID, path+f.Path)
				continue
			}
			doorCompareSchema(t, routeID, path+f.Path, f.Type, child)
		}
		for name := range node.Properties {
			if !doorHasField(typ, name) {
				t.Errorf("the contract's %q schema names %q, which the daemon does not read",
					routeID, name)
			}
		}
	case reflect.Slice, reflect.Array:
		if typ.Elem().Kind() != reflect.Struct {
			return
		}
		if node.Items == nil {
			t.Errorf("the contract's %q schema names no shape for %q, which the daemon reads item by item",
				routeID, path)
			return
		}
		doorCompareSchema(t, routeID, path+"[]", typ.Elem(), *node.Items)
	}
}

// doorHasField reports whether the daemon reads a field under that name.
func doorHasField(typ reflect.Type, name string) bool {
	for _, f := range doorDirectFields(typ) {
		if f.Path == name {
			return true
		}
	}
	return false
}

// TestTheDoorRefusesAMistypedField: for every field of every body, a body
// that carries the wrong type is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	s := newAPIServer(t, &fakeEngine{answer: "ok", model: "m"}, "secret")
	handler := s.Handler()
	for _, route := range doorBodies(t) {
		for _, f := range doorFields(route.Body, "") {
			_, wrong := doorValue(f.Type)
			if wrong == nil {
				continue
			}
			status, body := doorPost(t, handler, route.Path, doorBodyFor(t, route.Body, f.Path, wrong))
			want := "operation " + route.ID + " input: " + doorJSONPath(f.Path) + " has wrong type: got "
			got := doorErrorMessage(t, body)
			if status != http.StatusBadRequest {
				t.Errorf("the %q route: a mistyped %q answered %d %s, want 400", route.ID, f.Path, status, body)
				continue
			}
			if !strings.Contains(got, want) {
				t.Errorf("the %q route: a mistyped %q said %q, want it to contain %q",
					route.ID, f.Path, got, want)
				continue
			}
			if !strings.Contains(got, "want "+strings.Join(f.Words, "|")) {
				t.Errorf("the %q route: a mistyped %q said %q, want it to name the type %q",
					route.ID, f.Path, got, strings.Join(f.Words, "|"))
			}
		}
	}
}

// TestTheDoorRefusesANullField covers the arm the daemon's own decoder used
// to swallow: a null is not a value, and it is not a default either.
func TestTheDoorRefusesANullField(t *testing.T) {
	s := newAPIServer(t, &fakeEngine{answer: "ok", model: "m"}, "secret")
	handler := s.Handler()
	for _, route := range doorBodies(t) {
		for _, f := range doorFields(route.Body, "") {
			if len(f.Words) == 0 {
				continue
			}
			status, body := doorPost(t, handler, route.Path, doorBodyFor(t, route.Body, f.Path, nil))
			got := doorErrorMessage(t, body)
			if status != http.StatusBadRequest {
				t.Errorf("the %q route: a null %q answered %d %s, want 400", route.ID, f.Path, status, body)
				continue
			}
			want := "operation " + route.ID + " input: " + doorJSONPath(f.Path) +
				" has wrong type: got null, want " + strings.Join(f.Words, "|")
			if got != want {
				t.Errorf("the %q route: a null %q said %q, want %q", route.ID, f.Path, got, want)
			}
		}
	}
}

// TestTheDoorLetsAContractBodyThrough proves the door does not close the
// surface: a body that fits the contract reaches the handler.
func TestTheDoorLetsAContractBodyThrough(t *testing.T) {
	s := newAPIServer(t, &fakeEngine{answer: "ok", model: "m"}, "secret")
	handler := s.Handler()
	for _, route := range doorBodies(t) {
		status, body := doorPost(t, handler, route.Path, doorBodyFor(t, route.Body, "", nil))
		if status == http.StatusBadRequest && strings.Contains(body, "has wrong type") {
			t.Errorf("the %q route refused a body that fits the contract: %s", route.ID, body)
		}
	}
}

// TestTheDoorAllowsAFieldTheContractDoesNotName keeps the door the registry's
// door: a body is the stable SUBSET of what the daemon reads, so a caller may
// send a field the contract does not declare.
//
// The refusal is looked for by its WORDS ("is not allowed by schema"), not by
// the status code: a route may answer 400 for its own reasons (a message list
// with no usable content), and that is the surface's answer, not the door's.
func TestTheDoorAllowsAFieldTheContractDoesNotName(t *testing.T) {
	s := newAPIServer(t, &fakeEngine{answer: "ok", model: "m"}, "secret")
	handler := s.Handler()
	for _, route := range doorBodies(t) {
		_, body := doorPost(t, handler, route.Path, doorBodyFor(t, route.Body, "extra_field", 1))
		if strings.Contains(body, "is not allowed by schema") {
			t.Errorf("the %q route refused a body carrying an undeclared field: %s", route.ID, body)
		}
	}
}

// TestTheDoorRefusesABodyThatIsNotAnObject covers the root: a body that is
// not an object does not fit any row.
func TestTheDoorRefusesABodyThatIsNotAnObject(t *testing.T) {
	s := newAPIServer(t, &fakeEngine{answer: "ok", model: "m"}, "secret")
	handler := s.Handler()
	for _, route := range doorBodies(t) {
		status, body := doorPost(t, handler, route.Path, "[]")
		want := "operation " + route.ID + " input: $ has wrong type: got array, want object"
		got := doorErrorMessage(t, body)
		if status != http.StatusBadRequest {
			t.Errorf("the %q route: an array body answered %d %s, want 400", route.ID, status, body)
			continue
		}
		if got != want {
			t.Errorf("the %q route: an array body said %q, want %q", route.ID, got, want)
		}
	}
}

// TestTheOpenAISurfaceKeepsItsOwnRefusals proves the door did not take the
// surface's own answers with it: a message list with no usable content is
// still the surface's own refusal, and a body that does not fit the contract
// is still the OpenAI error envelope.
func TestTheOpenAISurfaceKeepsItsOwnRefusals(t *testing.T) {
	s := newAPIServer(t, &fakeEngine{answer: "ok", model: "m"}, "secret")
	handler := s.Handler()
	status, body := doorPost(t, handler, "/v1/chat/completions", `{"model":"m","messages":[]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("an empty message list answered %d %s, want 400", status, body)
	}
	if got := doorErrorMessage(t, body); got != "no usable message content" {
		t.Errorf("an empty message list said %q, want the surface's own \"no usable message content\"", got)
	}
	status, body = doorPost(t, handler, "/v1/responses", `{"model":"m","input":""}`)
	if status != http.StatusBadRequest {
		t.Fatalf("an empty input answered %d %s, want 400", status, body)
	}
	if got := doorErrorMessage(t, body); got != "no usable input content" {
		t.Errorf("an empty input said %q, want the surface's own \"no usable input content\"", got)
	}
	status, body = doorPost(t, handler, "/v1/chat/completions", `{"model":42}`)
	if status != http.StatusBadRequest {
		t.Fatalf("a mistyped body answered %d %s, want 400", status, body)
	}
	if !strings.Contains(body, "invalid_request_error") {
		t.Errorf("a mistyped body answered %s, want the OpenAI error envelope", body)
	}
	if !strings.Contains(body, "operation chat_completions input:") {
		t.Errorf("a mistyped body answered %s, want the registry's words inside it", body)
	}
}
