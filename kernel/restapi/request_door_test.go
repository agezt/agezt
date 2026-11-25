// SPDX-License-Identifier: MIT

package restapi

// The daemon's request door.
//
// The contract names what a request body looks like (kernel/contract/restapiapi)
// and the SDKs build their bodies from those rows. The door checks an arriving
// body against the contract's row BEFORE the handler reads it, and it refuses
// a mistyped body in the operation registry's own words:
//
//	operation <op> input: $.<field> has wrong type: got <got>, want <want>
//
// The tests below hold that door to three things: every field of every
// request row is checked, a body that fits the contract gets through
// untouched, and the refusal is the registry's message — not a lookalike.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/restapiapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// doorEndpoint is one endpoint that takes a request body: where the door is
// reachable and which row it measures against.
type doorEndpoint struct {
	ID   string
	Path string
	Row  restapiapi.Row
}

// doorEndpoints returns every endpoint that takes a request body.
//
// It is derived from the CONTRACT, not from a hand-written list: an endpoint
// the contract gives a request row is an endpoint the door must guard, and a
// new one that nobody wired behind the door fails the tests below.
func doorEndpoints(t *testing.T) []doorEndpoint {
	t.Helper()
	rows := map[string]restapiapi.Row{}
	for _, r := range restapiapi.RequestRows() {
		rows[r.Name] = r
	}
	var out []doorEndpoint
	for _, e := range restapiapi.Endpoints() {
		if e.Request == "" {
			continue
		}
		row, ok := rows[e.Request]
		if !ok {
			t.Fatalf("the %q endpoint names the request row %q, which the contract has no row for",
				e.ID, e.Request)
		}
		out = append(out, doorEndpoint{
			ID:   e.ID,
			Path: strings.ReplaceAll(e.Path, "{id}", "message-1"),
			Row:  row,
		})
	}
	if len(out) == 0 {
		t.Fatal("no endpoint takes a request body — the door has nothing to guard")
	}
	return out
}

// doorValue is a value that fits a field's declared type.
func doorValue(f restapiapi.Field) (any, bool) {
	switch f.Type {
	case restapiapi.TypeString:
		return "x", true
	case restapiapi.TypeBool:
		return true, true
	case restapiapi.TypeInt:
		return 7, true
	default:
		return nil, false
	}
}

// doorWrongValue is a value that does NOT fit a field's declared type, and the
// words the registry's validator says about it.
func doorWrongValue(f restapiapi.Field) (any, string, bool) {
	switch f.Type {
	case restapiapi.TypeString:
		return 42, "got number, want string", true
	case restapiapi.TypeBool:
		return "yes", "got string, want boolean", true
	case restapiapi.TypeInt:
		return "7", "got string, want number", true
	default:
		return nil, "", false
	}
}

// doorBody builds a request body that fits the row, except for the one field
// the caller mistypes.
func doorBody(t *testing.T, row restapiapi.Row, mistyped string, value any) string {
	t.Helper()
	body := map[string]any{}
	for _, f := range row.Fields {
		v, ok := doorValue(f)
		if !ok {
			continue
		}
		body[f.Name] = v
	}
	if mistyped != "" {
		body[mistyped] = value
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal the body: %v", err)
	}
	return string(raw)
}

// doorErrorMessage reads the `error.message` the surface answered with.
func doorErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the answer %s: %v", rec.Body.String(), err)
	}
	return body.Error.Message
}

// TestTheRequestDoorRefusesAMistypedField is the door's own test: for every
// field of every request row, a body that carries the wrong type is refused
// with the registry's words.
//
// It is also the guard that keeps the door whole: the endpoints come from the
// contract, so an endpoint the contract gives a request row but nobody wired
// behind the door fails here.
func TestTheRequestDoorRefusesAMistypedField(t *testing.T) {
	s, _ := mailboxServer(t, "tok")
	for _, e := range doorEndpoints(t) {
		for _, f := range e.Row.Fields {
			wrong, words, ok := doorWrongValue(f)
			if !ok {
				continue
			}
			rec := do(t, s, http.MethodPost, e.Path, doorBody(t, e.Row, f.Name, wrong), "tok")
			want := "operation " + e.ID + " input: $." + f.Name + " has wrong type: " + words
			if rec.Code != http.StatusBadRequest {
				t.Errorf("the %q endpoint: a mistyped %q answered %d %s, want 400",
					e.ID, f.Name, rec.Code, rec.Body.String())
				continue
			}
			if got := doorErrorMessage(t, rec); got != want {
				t.Errorf("the %q endpoint: a mistyped %q said %q, want %q",
					e.ID, f.Name, got, want)
			}
		}
	}
}

// TestTheRequestDoorRefusesANullField covers the arm the daemon's own decoder
// used to swallow: a null is not a value, and it is not a default either.
func TestTheRequestDoorRefusesANullField(t *testing.T) {
	s, _ := mailboxServer(t, "tok")
	for _, e := range doorEndpoints(t) {
		for _, f := range e.Row.Fields {
			if _, ok := doorValue(f); !ok {
				continue
			}
			rec := do(t, s, http.MethodPost, e.Path, doorBody(t, e.Row, f.Name, nil), "tok")
			want := "operation " + e.ID + " input: $." + f.Name + " has wrong type: got null, want " +
				schemaTypeWord(f.Type)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("the %q endpoint: a null %q answered %d %s, want 400",
					e.ID, f.Name, rec.Code, rec.Body.String())
				continue
			}
			if got := doorErrorMessage(t, rec); got != want {
				t.Errorf("the %q endpoint: a null %q said %q, want %q", e.ID, f.Name, got, want)
			}
		}
	}
}

// schemaTypeWord is the type the registry's validator names for a wire type.
func schemaTypeWord(t restapiapi.Type) string {
	switch t {
	case restapiapi.TypeString:
		return "string"
	case restapiapi.TypeBool:
		return "boolean"
	case restapiapi.TypeInt:
		return "number"
	default:
		return ""
	}
}

// TestTheRequestDoorLetsAContractBodyThrough proves the door does not close
// the surface: a body that fits the contract reaches the handler and gets the
// handler's own answer.
func TestTheRequestDoorLetsAContractBodyThrough(t *testing.T) {
	s, _ := mailboxServer(t, "tok")
	for _, e := range doorEndpoints(t) {
		rec := do(t, s, http.MethodPost, e.Path, doorBody(t, e.Row, "", nil), "tok")
		if rec.Code == http.StatusBadRequest && strings.Contains(doorErrorMessage(t, rec), "has wrong type") {
			t.Errorf("the %q endpoint refused a body that fits the contract: %s",
				e.ID, rec.Body.String())
		}
	}
}

// TestTheRequestDoorAllowsAFieldTheContractDoesNotName keeps the door the
// registry's door: a row is the stable SUBSET of a body, so a caller may send
// a field the contract does not declare.
func TestTheRequestDoorAllowsAFieldTheContractDoesNotName(t *testing.T) {
	s, _ := mailboxServer(t, "tok")
	for _, e := range doorEndpoints(t) {
		rec := do(t, s, http.MethodPost, e.Path, doorBody(t, e.Row, "extra_field", 1), "tok")
		if rec.Code == http.StatusBadRequest {
			t.Errorf("the %q endpoint refused a body carrying an undeclared field: %s",
				e.ID, rec.Body.String())
		}
	}
}

// TestTheRequestDoorRefusesABodyThatIsNotAnObject covers the root: a body
// that is not an object does not fit any row.
func TestTheRequestDoorRefusesABodyThatIsNotAnObject(t *testing.T) {
	s, _ := mailboxServer(t, "tok")
	for _, e := range doorEndpoints(t) {
		rec := do(t, s, http.MethodPost, e.Path, "[]", "tok")
		want := "operation " + e.ID + " input: $ has wrong type: got array, want object"
		if rec.Code != http.StatusBadRequest {
			t.Errorf("the %q endpoint: an array body answered %d %s, want 400",
				e.ID, rec.Code, rec.Body.String())
			continue
		}
		if got := doorErrorMessage(t, rec); got != want {
			t.Errorf("the %q endpoint: an array body said %q, want %q", e.ID, got, want)
		}
	}
}

// TestTheRequestDoorSpeaksTheRegistrysLanguage compares the door's refusal
// with what the operation registry's door would say for the same body — the
// registry wraps its schema error as `operation <op> input: <err>`
// (kernel/app/dispatch.go). Same schema, same validator, same words.
func TestTheRequestDoorSpeaksTheRegistrysLanguage(t *testing.T) {
	shape, ok := restapiapi.RequestSchema(restapiapi.EndpointRun)
	if !ok {
		t.Fatalf("the %q endpoint has no request schema", restapiapi.EndpointRun)
	}
	schemaErr := schema.ValidateJSON(shape, json.RawMessage(`{"intent":42}`))
	if schemaErr == nil {
		t.Fatal("the run request schema accepted a number for a string field")
	}
	registry := fmt.Errorf("operation %s input: %w", restapiapi.EndpointRun, schemaErr).Error()

	s, _ := mailboxServer(t, "tok")
	rec := do(t, s, http.MethodPost, "/api/v1/runs", `{"intent":42}`, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a mistyped run body answered %d %s, want 400", rec.Code, rec.Body.String())
	}
	if got := doorErrorMessage(t, rec); got != registry {
		t.Errorf("the door said %q, the registry says %q", got, registry)
	}
}

// TestTheRequestDoorKeepsTheSurfacesOwnRefusals proves the door did not take
// the surface's own answers with it: an empty body and a malformed body are
// still the errors the surface has always given.
func TestTheRequestDoorKeepsTheSurfacesOwnRefusals(t *testing.T) {
	s, _ := mailboxServer(t, "tok")
	cases := []struct {
		name string
		body string
		want string
	}{
		{"an empty body", "", "invalid JSON body: EOF"},
		{"a malformed body", "{not json", "invalid JSON body:"},
	}
	for _, c := range cases {
		rec := do(t, s, http.MethodPost, "/api/v1/runs", c.body, "tok")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d %s, want 400", c.name, rec.Code, rec.Body.String())
			continue
		}
		got := doorErrorMessage(t, rec)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s said %q, want it to contain %q", c.name, got, c.want)
		}
		if strings.Contains(got, "has wrong type") {
			t.Errorf("%s said %q, want the surface's own error, not the door's", c.name, got)
		}
	}
}
