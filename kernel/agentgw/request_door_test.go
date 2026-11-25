// SPDX-License-Identifier: MIT

package agentgw

// The gateway's request door.
//
// The contract names what a request body looks like (kernel/contract/agentgwapi)
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
//  4. the gateway's own refusals (a missing key, a missing capability) are
//     still the gateway's own.
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
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/agentgwapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/internal/testfixtures"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
)

// doorBody is one route that takes a request body: where the door is
// reachable, and which struct the daemon reads the body into.
type doorBody struct {
	ID   string
	Path string
	Body reflect.Type
}

// doorBodies are the gateway's request bodies.
//
// The list is derived from the CONTRACT — a route the contract gives a body
// is a route the door must guard — and each entry names the daemon's own
// struct, so the test states its expectations from the daemon's side.
func doorBodies(t *testing.T) []doorBody {
	t.Helper()
	structs := map[string]reflect.Type{
		agentgwapi.RouteEventbusPublish: reflect.TypeOf(eventPublishRequest{}),
		agentgwapi.RouteMemoryWrite:     reflect.TypeOf(memoryWriteRequest{}),
		agentgwapi.RouteLogWrite:        reflect.TypeOf(logWriteRequest{}),
		agentgwapi.RouteTokenCreate:     reflect.TypeOf(tokenCreateRequest{}),
		agentgwapi.RouteConfigSet:       reflect.TypeOf(ConfigSetRequest{}),
	}
	var out []doorBody
	for _, r := range agentgwapi.Routes() {
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
		t.Fatal("no route takes a request body — the door has nothing to guard")
	}
	return out
}

// doorValue is a value that fits a Go type, the value the registry's
// validator calls that type, and a value that does NOT fit it.
func doorValue(t reflect.Type) (valid any, wrong any, word string) {
	switch t.Kind() {
	case reflect.String:
		return "x", 42, "string"
	case reflect.Bool:
		return true, "yes", "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return 7, "7", "number"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			return []string{"x"}, "x", "array"
		}
		return []any{"x"}, "x", "array"
	case reflect.Map:
		if t.Elem().Kind() == reflect.String {
			return map[string]string{"k": "v"}, "x", "object"
		}
		return map[string]any{"k": "v"}, "x", "object"
	default:
		return nil, nil, ""
	}
}

// doorFields reads a struct's wire fields: name, Go type.
func doorFields(t reflect.Type) []struct {
	Name string
	Type reflect.Type
} {
	var out []struct {
		Name string
		Type reflect.Type
	}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, struct {
			Name string
			Type reflect.Type
		}{name, t.Field(i).Type})
	}
	return out
}

// doorBodyFor builds a body that fits the struct, except for the one field
// the caller mistypes (mistyped != "").
func doorBodyFor(t *testing.T, typ reflect.Type, mistyped string, value any) string {
	t.Helper()
	body := map[string]any{}
	for _, f := range doorFields(typ) {
		valid, _, _ := doorValue(f.Type)
		if valid == nil {
			continue
		}
		body[f.Name] = valid
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

// doorGateway is a gateway with every dependency the body's routes need: the
// bus, the memory, the roster and the config center.
func doorGateway(t *testing.T) (*Gateway, string) {
	t.Helper()
	dir := t.TempDir()
	gw := NewGateway(DefaultGatewayConfig(dir))

	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(func() { b.Close() })

	memStore, err := memory.Open(dir)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	mem := memory.NewManager(memoryapi.Deps{Store: memStore, Publisher: testfixtures.BusPublisher{B: b}})
	ros, err := roster.Open(dir)
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	gw.Attach(b, mem, ros)

	center, err := configcenter.New(configcenter.DefaultConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("configcenter.New: %v", err)
	}
	t.Cleanup(func() { center.Close() })
	gw.SetConfigCenter(center)

	token, err := gw.tokenMgr.CreateToken(&TokenClaims{
		RunID:     "door-run",
		Caps:      []string{"eventbus.publish", "memory.write", "log.write", "config.write"},
		MaxRate:   120,
		MaxBurst:  20,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return gw, token
}

// doorPost posts a body to the gateway and answers the status and the body.
func doorPost(t *testing.T, handler http.Handler, token, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// doorErrorMessage reads the `error.message` the gateway answered with.
func doorErrorMessage(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode the answer %s: %v", body, err)
	}
	return payload.Error.Message
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
		schema, ok := agentgwapi.RequestSchema(route.ID)
		if !ok {
			t.Errorf("the %q route has no schema — the door cannot measure its body", route.ID)
			continue
		}
		var doc struct {
			Properties map[string]struct {
				Type  string `json:"type"`
				Items *struct {
					Type string `json:"type"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(schema, &doc); err != nil {
			t.Fatalf("the %q route's schema is not readable: %v", route.ID, err)
		}
		named := map[string]bool{}
		for _, f := range doorFields(route.Body) {
			named[f.Name] = true
			prop, ok := doc.Properties[f.Name]
			if !ok {
				t.Errorf("the contract's %q schema does not name %q, which the daemon reads", route.ID, f.Name)
				continue
			}
			_, _, word := doorValue(f.Type)
			if prop.Type != word {
				t.Errorf("the contract's %q schema names %q as %q, the daemon reads it as %q",
					route.ID, f.Name, prop.Type, word)
			}
		}
		for name := range doc.Properties {
			if !named[name] {
				t.Errorf("the contract's %q schema names %q, which the daemon does not read", route.ID, name)
			}
		}
	}
}

// TestTheDoorRefusesAMistypedField: for every field of every body, a body
// that carries the wrong type is refused with the registry's words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	gw, token := doorGateway(t)
	handler := gw.Handler()
	for _, route := range doorBodies(t) {
		for _, f := range doorFields(route.Body) {
			_, wrong, word := doorValue(f.Type)
			if wrong == nil {
				continue
			}
			status, body := doorPost(t, handler, token, route.Path, doorBodyFor(t, route.Body, f.Name, wrong))
			want := "operation " + route.ID + " input: $." + f.Name + " has wrong type: got "
			got := doorErrorMessage(t, body)
			if status != http.StatusBadRequest {
				t.Errorf("the %q route: a mistyped %q answered %d %s, want 400", route.ID, f.Name, status, body)
				continue
			}
			if !strings.Contains(got, want) {
				t.Errorf("the %q route: a mistyped %q said %q, want it to contain %q",
					route.ID, f.Name, got, want)
				continue
			}
			if !strings.Contains(got, "want "+word) {
				t.Errorf("the %q route: a mistyped %q said %q, want it to name the type %q",
					route.ID, f.Name, got, word)
			}
		}
	}
}

// TestTheDoorRefusesANullField covers the arm the daemon's own decoder used
// to swallow: a null is not a value, and it is not a default either.
func TestTheDoorRefusesANullField(t *testing.T) {
	gw, token := doorGateway(t)
	handler := gw.Handler()
	for _, route := range doorBodies(t) {
		for _, f := range doorFields(route.Body) {
			_, _, word := doorValue(f.Type)
			if word == "" {
				continue
			}
			status, body := doorPost(t, handler, token, route.Path, doorBodyFor(t, route.Body, f.Name, nil))
			got := doorErrorMessage(t, body)
			if status != http.StatusBadRequest {
				t.Errorf("the %q route: a null %q answered %d %s, want 400", route.ID, f.Name, status, body)
				continue
			}
			want := "operation " + route.ID + " input: $." + f.Name + " has wrong type: got null, want " + word
			if got != want {
				t.Errorf("the %q route: a null %q said %q, want %q", route.ID, f.Name, got, want)
			}
		}
	}
}

// TestTheDoorLetsAContractBodyThrough proves the door does not close the
// gateway: a body that fits the contract reaches the handler.
func TestTheDoorLetsAContractBodyThrough(t *testing.T) {
	gw, token := doorGateway(t)
	handler := gw.Handler()
	for _, route := range doorBodies(t) {
		status, body := doorPost(t, handler, token, route.Path, doorBodyFor(t, route.Body, "", nil))
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
// the status code: a route may answer 400 for its own reasons (the token
// route refuses an unknown capability), and that is the gateway's answer, not
// the door's.
func TestTheDoorAllowsAFieldTheContractDoesNotName(t *testing.T) {
	gw, token := doorGateway(t)
	handler := gw.Handler()
	for _, route := range doorBodies(t) {
		_, body := doorPost(t, handler, token, route.Path, doorBodyFor(t, route.Body, "extra_field", 1))
		if strings.Contains(body, "is not allowed by schema") {
			t.Errorf("the %q route refused a body carrying an undeclared field: %s", route.ID, body)
		}
	}
}

// TestTheDoorRefusesABodyThatIsNotAnObject covers the root: a body that is
// not an object does not fit any row.
func TestTheDoorRefusesABodyThatIsNotAnObject(t *testing.T) {
	gw, token := doorGateway(t)
	handler := gw.Handler()
	for _, route := range doorBodies(t) {
		status, body := doorPost(t, handler, token, route.Path, "[]")
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

// TestTheGatewayKeepsItsOwnRefusals proves the door did not take the
// gateway's own answers with it: a missing key is still the gateway's
// "key is required", and a body that does not fit the contract is still the
// gateway's INVALID_REQUEST envelope.
func TestTheGatewayKeepsItsOwnRefusals(t *testing.T) {
	gw, token := doorGateway(t)
	handler := gw.Handler()
	status, body := doorPost(t, handler, token, "/v1/config", `{}`)
	if status != http.StatusBadRequest {
		t.Fatalf("an empty config body answered %d %s, want 400", status, body)
	}
	if !strings.Contains(body, "MISSING_KEY") {
		t.Errorf("an empty config body answered %s, want the gateway's own MISSING_KEY", body)
	}
	status, body = doorPost(t, handler, token, "/v1/eventbus/publish", `{"event":42}`)
	if status != http.StatusBadRequest {
		t.Fatalf("a mistyped body answered %d %s, want 400", status, body)
	}
	if !strings.Contains(body, "INVALID_REQUEST") {
		t.Errorf("a mistyped body answered %s, want the gateway's INVALID_REQUEST envelope", body)
	}
	if !strings.Contains(body, "operation eventbus_publish input:") {
		t.Errorf("a mistyped body answered %s, want the registry's words inside it", body)
	}
}
