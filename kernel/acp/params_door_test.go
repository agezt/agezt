// SPDX-License-Identifier: MIT

package acp

// The ACP surface's door guards.
//
// The surface is named in the contract (kernel/contract/acpserverapi) and the
// door measures an arriving params object against the shape the contract
// names, BEFORE the handler reads it. The tests below hold that line: a
// mistyped field, a null field and a params object that is not an object at
// all are all refused with the operation registry's words — and a body that
// fits the contract still reaches the handler.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/acpserverapi"
)

// doorError runs one request and returns the error message the surface
// answered, or "" when it answered with a result.
func doorError(t *testing.T, method string, params any) string {
	t.Helper()
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 reply for %q, got %d", method, len(out))
	}
	e, ok := out[0]["error"].(map[string]any)
	if !ok {
		return ""
	}
	if code, _ := e["code"].(float64); int(code) != codeInvalidParams {
		t.Errorf("the %q method answered the code %v, want %d", method, e["code"], codeInvalidParams)
	}
	msg, _ := e["message"].(string)
	return msg
}

// doorErrorWithoutParams runs one request that carries NO params key at all —
// the message an absent params object looks like on the wire, which is not the
// same as a params object that is JSON null.
func doorErrorWithoutParams(t *testing.T, method string) string {
	t.Helper()
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method,
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 reply for %q, got %d", method, len(out))
	}
	e, ok := out[0]["error"].(map[string]any)
	if !ok {
		return ""
	}
	msg, _ := e["message"].(string)
	return msg
}

// TestTheServerHoldsTheContractToItself: the shapes the server reads are the
// shapes the contract names — the same fields, with the same types, in both
// directions.
//
// This is the check the server makes when it is built; here it is made
// explicit, so a drift is a failing test rather than a boot-time panic alone.
func TestTheServerHoldsTheContractToItself(t *testing.T) {
	if err := acpserverapi.CheckShape(acpserverapi.MethodSessionNew, reflect.TypeOf(newSessionParams{})); err != nil {
		t.Errorf("the contract and the server disagree about the %q params: %v",
			acpserverapi.MethodSessionNew, err)
	}
	if err := acpserverapi.CheckShape(acpserverapi.MethodSessionPrompt, reflect.TypeOf(promptParams{})); err != nil {
		t.Errorf("the contract and the server disagree about the %q params: %v",
			acpserverapi.MethodSessionPrompt, err)
	}
}

// TestTheServerRefusesToStartWhenTheContractDisagrees: the boot-time ratchet
// is real — a shape the server does not read is caught, so the daemon stops
// instead of quietly measuring a body it never reads.
//
// The ratchet is asked with the shapes the server ACTUALLY reads; here it is
// asked with a shape that does not fit, so the guard on the ratchet itself
// can be exercised.
func TestTheServerRefusesToStartWhenTheContractDisagrees(t *testing.T) {
	type wrongParams struct {
		Cwd int `json:"cwd"`
	}
	if err := checkContract(reflect.TypeOf(wrongParams{}), reflect.TypeOf(promptParams{})); err == nil {
		t.Error("the ratchet accepted a session/new shape the server does not read")
	}
	type wrongPrompt struct {
		SessionID string `json:"sessionId"`
		Prompt    string `json:"prompt"`
	}
	if err := checkContract(reflect.TypeOf(newSessionParams{}), reflect.TypeOf(wrongPrompt{})); err == nil {
		t.Error("the ratchet accepted a session/prompt shape the server does not read")
	}
}

// TestTheDoorRefusesAMistypedField: for every field of every params shape, a
// params object that carries the wrong type is refused with the registry's
// words.
func TestTheDoorRefusesAMistypedField(t *testing.T) {
	cases := []struct {
		method string
		params any
		want   string
	}{
		{
			method: acpserverapi.MethodSessionNew,
			params: map[string]any{"cwd": 42},
			want:   "operation session_new input: $.cwd has wrong type: got number, want string",
		},
		{
			method: acpserverapi.MethodSessionNew,
			params: map[string]any{"cwd": true},
			want:   "operation session_new input: $.cwd has wrong type: got boolean, want string",
		},
		{
			method: acpserverapi.MethodSessionNew,
			params: map[string]any{"cwd": []any{}},
			want:   "operation session_new input: $.cwd has wrong type: got array, want string",
		},
		{
			method: acpserverapi.MethodSessionNew,
			params: map[string]any{"cwd": map[string]any{}},
			want:   "operation session_new input: $.cwd has wrong type: got object, want string",
		},
		{
			method: acpserverapi.MethodSessionPrompt,
			params: map[string]any{"sessionId": 42, "prompt": []any{}},
			want:   "operation session_prompt input: $.sessionId has wrong type: got number, want string",
		},
		{
			method: acpserverapi.MethodSessionPrompt,
			params: map[string]any{"sessionId": "sess-1", "prompt": "hello"},
			want:   "operation session_prompt input: $.prompt has wrong type: got string, want array",
		},
		{
			method: acpserverapi.MethodSessionPrompt,
			params: map[string]any{"sessionId": "sess-1", "prompt": []any{map[string]any{"type": 42, "text": "x"}}},
			want:   "operation session_prompt input: $.prompt[0].type has wrong type: got number, want string",
		},
		{
			method: acpserverapi.MethodSessionPrompt,
			params: map[string]any{"sessionId": "sess-1", "prompt": []any{map[string]any{"type": "text", "text": 42}}},
			want:   "operation session_prompt input: $.prompt[0].text has wrong type: got number, want string",
		},
	}
	for _, tc := range cases {
		got := doorError(t, tc.method, tc.params)
		if got == "" {
			t.Errorf("the %q method accepted %v, want the refusal %q", tc.method, tc.params, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("the %q method said %q, want %q", tc.method, got, tc.want)
		}
	}
}

// TestTheDoorRefusesANullField covers the arm the decoder used to swallow: a
// null is not a value, and it is not a default either.
func TestTheDoorRefusesANullField(t *testing.T) {
	cases := []struct {
		method string
		params any
		want   string
	}{
		{
			method: acpserverapi.MethodSessionNew,
			params: map[string]any{"cwd": nil},
			want:   "operation session_new input: $.cwd has wrong type: got null, want string",
		},
		{
			method: acpserverapi.MethodSessionPrompt,
			params: map[string]any{"sessionId": nil, "prompt": []any{}},
			want:   "operation session_prompt input: $.sessionId has wrong type: got null, want string",
		},
		{
			method: acpserverapi.MethodSessionPrompt,
			params: map[string]any{"sessionId": "sess-1", "prompt": nil},
			want:   "operation session_prompt input: $.prompt has wrong type: got null, want array",
		},
		{
			method: acpserverapi.MethodSessionPrompt,
			params: map[string]any{"sessionId": "sess-1", "prompt": []any{map[string]any{"type": "text", "text": nil}}},
			want:   "operation session_prompt input: $.prompt[0].text has wrong type: got null, want string",
		},
	}
	for _, tc := range cases {
		got := doorError(t, tc.method, tc.params)
		if got != tc.want {
			t.Errorf("the %q method said %q, want %q", tc.method, got, tc.want)
		}
	}
}

// TestTheDoorRefusesAParamsThatIsNotAnObject: the params of a method that
// takes an object must BE an object — a number, a string, a list or a null
// is refused with the registry's words.
func TestTheDoorRefusesAParamsThatIsNotAnObject(t *testing.T) {
	cases := []struct {
		params any
		want   string
	}{
		{params: 42, want: "operation session_new input: $ has wrong type: got number, want object"},
		{params: "hello", want: "operation session_new input: $ has wrong type: got string, want object"},
		{params: []any{}, want: "operation session_new input: $ has wrong type: got array, want object"},
		{params: nil, want: "operation session_new input: $ has wrong type: got null, want object"},
	}
	for _, tc := range cases {
		got := doorError(t, acpserverapi.MethodSessionNew, tc.params)
		if got != tc.want {
			t.Errorf("the session/new method said %q, want %q", got, tc.want)
		}
	}
}

// TestTheRefusalNoLongerNamesAGoType: the refusal is the registry's, and the
// registry names a FIELD — not a Go struct of an implementation detail.
//
// Before the door, the answer was "json: cannot unmarshal number into Go
// struct field newSessionParams.cwd of type string", which named a private
// type of the server and nothing a caller could act on.
func TestTheRefusalNoLongerNamesAGoType(t *testing.T) {
	got := doorError(t, acpserverapi.MethodSessionNew, map[string]any{"cwd": 42})
	for _, leak := range []string{"newSessionParams", "promptParams", "contentBlock", "cannot unmarshal"} {
		if strings.Contains(got, leak) {
			t.Errorf("the refusal %q names %q — it must name the field, not the Go type", got, leak)
		}
	}
}

// TestTheDoorLetsAContractBodyThrough proves the door does not close the
// surface: params that fit the contract reach the handler.
func TestTheDoorLetsAContractBodyThrough(t *testing.T) {
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": acpserverapi.MethodSessionNew,
		"params": map[string]any{"cwd": "/work"},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(out))
	}
	if out[0]["error"] != nil {
		t.Fatalf("the door refused params that fit the contract: %v", out[0]["error"])
	}
	res, _ := out[0]["result"].(map[string]any)
	if sid, _ := res["sessionId"].(string); sid == "" {
		t.Errorf("the session was not opened: %v", out[0])
	}
}

// TestTheDoorAllowsAFieldTheContractDoesNotName keeps the door the registry's
// door: a body is the stable SUBSET of what the daemon reads, so a caller may
// send a field the contract does not declare.
//
// An ACP client sends `mcpServers` to `session/new`, and the server does not
// read it — so the contract does not name it, and the door does not refuse
// it.
func TestTheDoorAllowsAFieldTheContractDoesNotName(t *testing.T) {
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": acpserverapi.MethodSessionNew,
		"params": map[string]any{"cwd": "/work", "mcpServers": []any{}},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(out))
	}
	if out[0]["error"] != nil {
		t.Errorf("the door refused params carrying an undeclared field: %v", out[0]["error"])
	}
}

// TestTheDoorLeavesPresenceToTheHandler: a method with NO params at all is
// not a type error — the door does not measure it, and the handler decides
// what a missing field means.
//
// `session/new` without params opens a session with no working directory
// (the handler's business), and `session/prompt` without params is refused by
// the handler — not by the door, and not with a schema's words.
func TestTheDoorLeavesPresenceToTheHandler(t *testing.T) {
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": acpserverapi.MethodSessionNew,
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(out))
	}
	if out[0]["error"] != nil {
		t.Errorf("the door refused params that were not there: %v", out[0]["error"])
	}
	got := doorErrorWithoutParams(t, acpserverapi.MethodSessionPrompt)
	if got == "" {
		t.Fatal("a prompt with no params was accepted")
	}
	if strings.Contains(got, "has wrong type") {
		t.Errorf("the prompt without params was refused with a schema's words (%q) — presence is the handler's business", got)
	}
}

// TestTheDoorMeasuresBeforeTheHandlerReads: the refusal must come before the
// surface does any work — a prompt with a mistyped session id must not reach
// the session lookup.
//
// The lookup's own refusal is a different sentence ("unknown sessionId: …"),
// so the two cannot be confused.
func TestTheDoorMeasuresBeforeTheHandlerReads(t *testing.T) {
	got := doorError(t, acpserverapi.MethodSessionPrompt, map[string]any{
		"sessionId": 42, "prompt": []any{map[string]any{"type": "text", "text": "hi"}},
	})
	if !strings.HasPrefix(got, "operation session_prompt input:") {
		t.Errorf("the prompt said %q, want the door's sentence", got)
	}
	if strings.Contains(got, "unknown sessionId") {
		t.Errorf("the mistyped session id reached the session lookup: %q", got)
	}
}

// TestTheDoorReadsTheContractItself: the door measures the CONTRACT's shape,
// not a copy of it — so a contract that changes its shape changes what the
// door accepts.
//
// The guard is proved by the schema the contract names, read back through the
// contract itself.
func TestTheDoorReadsTheContractItself(t *testing.T) {
	shape, ok := acpserverapi.ParamsSchema(acpserverapi.MethodSessionNew)
	if !ok {
		t.Fatal("the contract names no shape for the session/new params")
	}
	var doc struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(shape, &doc); err != nil {
		t.Fatalf("the contract's shape is not readable: %v", err)
	}
	cwd, ok := doc.Properties["cwd"]
	if !ok {
		t.Fatalf("the contract's shape names no cwd: %s", shape)
	}
	if cwd.Type != "string" {
		t.Errorf("the contract names cwd as %q, want string", cwd.Type)
	}
}
