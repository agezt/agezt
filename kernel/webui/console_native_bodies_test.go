// SPDX-License-Identifier: MIT

package webui

// The console's own request bodies.
//
// The console's own endpoint list (kernel/contract/opapi/native.go) names each
// body's shape, and the console checks an arriving body against it before the
// handler reads it (request_door.go). These tests hold that line:
//
//   - every console-native endpoint that reads a body has a schema, and every
//     schema in the list belongs to an endpoint that reads one — a schema
//     nobody checks is a promise nobody keeps;
//   - the schema and the struct the handler reads name the SAME fields, in
//     both directions — a field the contract names and the handler ignores is
//     a field a caller was told is accepted and it is not;
//   - a mistyped body is refused with the operation registry's words, and a
//     body that fits the contract gets through.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// consoleNativeBodies are the console's own endpoints that take a request
// body, with the struct the handler reads it into.
//
// The endpoint list is the contract; this table is the console's half, and the
// ratchet below holds the two to each other. An endpoint that reads a body and
// is missing here cannot be checked by the door, so the door would be a hole
// the table cannot see.
func consoleNativeBodies() map[opapi.NativeRouteName]reflect.Type {
	return map[opapi.NativeRouteName]reflect.Type{
		opapi.NativeLogin:         reflect.TypeOf(loginBody{}),
		opapi.NativeRollbackApply: reflect.TypeOf(rollbackApplyBody{}),
		opapi.NativeFilesMkdir:    reflect.TypeOf(fileMkdirBody{}),
		opapi.NativeFilesRename:   reflect.TypeOf(fileRenameBody{}),
		opapi.NativeFilesDelete:   reflect.TypeOf(fileDeleteBody{}),
		opapi.NativeTTS:           reflect.TypeOf(ttsBody{}),
	}
}

// consoleNativeBodyPaths are the paths the console answers those endpoints at,
// so the door's own test can reach them.
//
// The path is READ from the contract (the console's own endpoint list), never
// written here: the console's rule is that a path is written once in the whole
// tree, and a test is part of the tree.
func consoleNativeBodyPaths() map[opapi.NativeRouteName]string {
	out := map[opapi.NativeRouteName]string{}
	for name := range consoleNativeBodies() {
		out[name] = opapi.ConsoleNativePath(name) + "?token=secret"
	}
	return out
}

// TestTheConsoleNativeBodiesAgreeWithTheContract is the ratchet: the console's
// own endpoint list and the console's own request structs are one set.
//
//   - an endpoint the console reads a body for has a schema in the contract;
//   - a schema in the contract belongs to an endpoint the console reads a body
//     for (a schema nobody checks is a promise nobody keeps);
//   - the schema's properties and the struct's fields name the SAME fields, in
//     both directions;
//   - and the schema's property TYPE is the type the handler reads the field
//     as — a schema that calls a boolean a string would refuse the body the
//     console actually accepts.
func TestTheConsoleNativeBodiesAgreeWithTheContract(t *testing.T) {
	bodies := consoleNativeBodies()
	for name, typ := range bodies {
		shape, ok := opapi.ConsoleNativeBodySchema(name)
		if !ok {
			t.Errorf("the console reads a body for the console-native endpoint %q, but the contract names no schema for it",
				name)
			continue
		}
		var doc struct {
			Properties map[string]struct {
				Type string `json:"type"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(shape, &doc); err != nil {
			t.Errorf("the console-native endpoint %q declares an unreadable schema: %v", name, err)
			continue
		}
		read := jsonFields(typ)
		for field, wire := range read {
			prop, ok := doc.Properties[field]
			if !ok {
				t.Errorf("the console-native endpoint %q reads %q, which the contract's schema does not name",
					name, field)
				continue
			}
			if prop.Type != wire {
				t.Errorf("the contract's schema for the console-native endpoint %q calls %q a %q, but the console reads it as a %q",
					name, field, prop.Type, wire)
			}
		}
		for field := range doc.Properties {
			if _, ok := read[field]; !ok {
				t.Errorf("the contract's schema for the console-native endpoint %q names %q, which the console does not read",
					name, field)
			}
		}
	}
	for _, route := range opapi.ConsoleNativeRoutes() {
		if len(route.Body) == 0 {
			continue
		}
		if _, ok := bodies[route.Name]; !ok {
			t.Errorf("the contract names a body for the console-native endpoint %q, but the console does not read one — "+
				"the schema is never checked", route.Name)
		}
	}
}

// jsonFields reads the wire names of a struct's fields, with the wire type the
// handler reads each one as.
func jsonFields(t reflect.Type) map[string]string {
	out := map[string]string{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = wireType(t.Field(i).Type)
	}
	return out
}

// wireType is the JSON type a Go field is read as on the wire.
func wireType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "number"
	case reflect.Float32, reflect.Float64:
		return "number"
	default:
		return ""
	}
}

// wrongValue is a value that does NOT fit the wire type, and the words the
// registry's validator says about it.
func wrongValue(wire string) (string, string, bool) {
	switch wire {
	case "string":
		return "42", "got number, want string", true
	case "boolean":
		return `"yes"`, "got string, want boolean", true
	case "number":
		return `"7"`, "got string, want number", true
	default:
		return "", "", false
	}
}

// TestTheConsoleDoorRefusesAMistypedBody is the door's own test: for every
// console-native endpoint that takes a body, a body that carries the wrong
// type is refused with the operation registry's words.
//
// The expectation is stated HERE — the console's own struct says what it
// reads — and not read back from the contract's schema, so a schema that
// mistyped a field cannot agree with itself and pass.
func TestTheConsoleDoorRefusesAMistypedBody(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	s.SetPasswordFn(func() string { return "hunter2" })
	s.SetSynthesizer(&stubSynthesizer{audio: []byte("x"), mime: "audio/mpeg"})
	for name, typ := range consoleNativeBodies() {
		path, ok := consoleNativeBodyPaths()[name]
		if !ok {
			t.Errorf("the console-native endpoint %q reads a body, but the test does not know its path", name)
			continue
		}
		for field, wire := range jsonFields(typ) {
			wrong, words, ok := wrongValue(wire)
			if !ok {
				continue
			}
			rec := httpJSON(t, s.Handler(), http.MethodPost, path, `{"`+field+`":`+wrong+`}`)
			want := "operation " + string(name) + " input: $." + field + " has wrong type: " + words
			if rec.Code != http.StatusBadRequest {
				t.Errorf("the console-native endpoint %q: a mistyped %q answered %d %s, want 400",
					name, field, rec.Code, rec.Body.String())
				continue
			}
			if got := consoleErrorMessage(t, rec); got != want {
				t.Errorf("the console-native endpoint %q: a mistyped %q said %q, want %q",
					name, field, got, want)
			}
		}
	}
}

// TestTheConsoleDoorRefusesANullField covers the arm the daemon's own decoder
// used to swallow: a null is not a value, and it is not a default either.
func TestTheConsoleDoorRefusesANullField(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	s.SetPasswordFn(func() string { return "hunter2" })
	s.SetSynthesizer(&stubSynthesizer{audio: []byte("x"), mime: "audio/mpeg"})
	for name, typ := range consoleNativeBodies() {
		path, ok := consoleNativeBodyPaths()[name]
		if !ok {
			continue
		}
		for field, wire := range jsonFields(typ) {
			var words string
			switch wire {
			case "string":
				words = "got null, want string"
			case "boolean":
				words = "got null, want boolean"
			case "number":
				words = "got null, want number"
			default:
				continue
			}
			rec := httpJSON(t, s.Handler(), http.MethodPost, path, `{"`+field+`":null}`)
			want := "operation " + string(name) + " input: $." + field + " has wrong type: " + words
			if rec.Code != http.StatusBadRequest {
				t.Errorf("the console-native endpoint %q: a null %q answered %d %s, want 400",
					name, field, rec.Code, rec.Body.String())
				continue
			}
			if got := consoleErrorMessage(t, rec); got != want {
				t.Errorf("the console-native endpoint %q: a null %q said %q, want %q", name, field, got, want)
			}
		}
	}
}

// TestTheConsoleDoorLetsAContractBodyThrough proves the door does not close
// the console: a body that fits the contract reaches the handler and gets the
// handler's own answer.
func TestTheConsoleDoorLetsAContractBodyThrough(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	s.SetPasswordFn(func() string { return "hunter2" })
	s.SetSynthesizer(&stubSynthesizer{audio: []byte("x"), mime: "audio/mpeg"})
	for name, typ := range consoleNativeBodies() {
		path, ok := consoleNativeBodyPaths()[name]
		if !ok {
			continue
		}
		body := map[string]any{}
		for field, wire := range jsonFields(typ) {
			switch wire {
			case "string":
				body[field] = "x"
			case "boolean":
				body[field] = false
			case "number":
				body[field] = 7
			}
		}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal the body: %v", err)
		}
		rec := httpJSON(t, s.Handler(), http.MethodPost, path, string(raw))
		if rec.Code == http.StatusBadRequest && strings.Contains(consoleErrorMessage(t, rec), "has wrong type") {
			t.Errorf("the console-native endpoint %q refused a body that fits the contract: %s",
				name, rec.Body.String())
		}
	}
}

// consoleErrorMessage reads the `error` the console answered with. An answer
// that is not the console's JSON envelope is reported as it came, so a
// failure says what the console actually said.
func consoleErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error == "" {
		return strings.TrimSpace(rec.Body.String())
	}
	return body.Error
}
