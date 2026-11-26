// SPDX-License-Identifier: MIT

// Package acpserverapi is the contract of the daemon's ACP server surface.
//
// The daemon speaks the Agent Client Protocol (SPEC-15 §3) to IDEs: the editor
// spawns the process and calls `initialize` → `session/new` → `session/prompt`
// over JSON-RPC 2.0 on stdio. Every one of those calls carries a `params`
// object — and until now nothing said what that object may contain.
//
// The params were read into whatever struct the handler happened to use:
//
//	type newSessionParams struct{ Cwd string `json:"cwd"` }
//	type promptParams struct {
//	    SessionID string         `json:"sessionId"`
//	    Prompt    []contentBlock `json:"prompt"`
//	}
//
// so what happened to a mistyped body was Go's encoding/json, and the answer
// named a Go struct — a private type of an implementation detail:
//
//	{"cwd":42}          → "invalid params: json: cannot unmarshal number into Go struct field newSessionParams.cwd of type string"
//	{"prompt":"hello"}  → "invalid params: json: cannot unmarshal string into Go struct field promptParams.prompt of type []acp.contentBlock"
//
// and a **null** was accepted: `{"cwd":null}` became the empty string, and a
// null prompt block became an empty block.
//
// This package holds the surface as data:
//
//   - Methods() is the roster of the surface: the wire method, the operation
//     name the refusal carries, and the JSON Schema of the params the method
//     takes.
//   - ParamsSchema() is the schema of one method's params — the shape the
//     server's door measures an arriving params object against.
//   - CheckShape() holds a schema to the Go type the server reads it as.
//
// Three rules keep the schemas the operation registry's shape:
//
//  1. A schema names TYPES, not presence. The door checks types; a missing
//     field stays the handler's business.
//  2. A schema allows an undeclared field. A body is the stable SUBSET of
//     what the daemon reads — an ACP client sends `mcpServers` to
//     `session/new`, and the server does not read it.
//  3. The type words are the registry's: an integer is a "number", a boolean
//     is a "boolean".
//
// The surface is not part of the SDK contract: the SDKs speak `/api/v1`, and
// the callers of this surface are the editors that drive the daemon.
package acpserverapi

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/agezt/agezt/kernel/contract/jsonshape"
)

// Method is one method of the ACP server surface.
type Method struct {
	// Op is the operation name the door's refusal carries:
	// "operation session_new input: $.cwd has wrong type: got number, want string".
	//
	// It is the registry's grammar — an identifier, not a path — so the
	// operation reads the way every other operation reads.
	Op string
	// Name is the JSON-RPC method on the wire ("session/new").
	Name string
	// Body is the JSON Schema of the params the method takes, empty when the
	// method reads none.
	Body json.RawMessage
}

// The wire methods of the ACP server surface.
const (
	// MethodInitialize negotiates the protocol version. The server answers
	// with its own capabilities and READS NOTHING of the params — so the
	// contract names no body for it.
	MethodInitialize = "initialize"
	// MethodSessionNew opens a session rooted at a working directory.
	MethodSessionNew = "session/new"
	// MethodSessionPrompt runs a prompt inside an open session.
	MethodSessionPrompt = "session/prompt"
	// MethodSessionCancel cancels a running prompt. It is a NOTIFICATION:
	// it carries no id and gets no answer.
	MethodSessionCancel = "session/cancel"
	// MethodCancelNotification is the namespaced spelling of the same
	// cancellation, sent by clients that namespace what they do not own.
	MethodCancelNotification = "$/cancelNotification"
)

// The operation names the refusal carries.
const (
	// OpInitialize is the operation of the `initialize` method.
	OpInitialize = "initialize"
	// OpSessionNew is the operation of the `session/new` method.
	OpSessionNew = "session_new"
	// OpSessionPrompt is the operation of the `session/prompt` method.
	OpSessionPrompt = "session_prompt"
	// OpSessionCancel is the operation of the `session/cancel` method.
	OpSessionCancel = "session_cancel"
	// OpCancelNotification is the operation of the `$/cancelNotification`
	// method.
	OpCancelNotification = "cancel_notification"
)

// methods is the surface's method table.
func methods() []Method {
	return []Method{
		{
			// The server reads none of the initialize params: it answers
			// with its own protocol version and capabilities.
			Op: OpInitialize, Name: MethodInitialize,
		},
		{
			Op: OpSessionNew, Name: MethodSessionNew,
			Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
				`"cwd":{"type":"string"}}}`),
		},
		{
			Op: OpSessionPrompt, Name: MethodSessionPrompt,
			Body: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` +
				`"sessionId":{"type":"string"},"prompt":{"type":"array","items":{"type":"object",` +
				`"additionalProperties":true,"properties":{"type":{"type":"string"},"text":{"type":"string"}}}}}}`),
		},
		{
			// A cancellation notification carries a session id the server
			// does not read at this version: the per-prompt context handles
			// the cancellation. So the contract names no body for it.
			Op: OpSessionCancel, Name: MethodSessionCancel,
		},
		{
			Op: OpCancelNotification, Name: MethodCancelNotification,
		},
	}
}

// Methods returns the method table of the ACP server surface.
func Methods() []Method { return methods() }

// Lookup returns the method the surface answers at a wire method name.
//
// It answers false for a method the contract does not know — so a caller can
// tell "this method takes no params" from "this method is not in the
// contract".
func Lookup(name string) (Method, bool) {
	for _, m := range Methods() {
		if m.Name == name {
			return m, true
		}
	}
	return Method{}, false
}

// ParamsSchema returns the JSON Schema of one method's params.
//
// It answers false for a method that takes none — and for a method the
// contract does not know.
func ParamsSchema(name string) (json.RawMessage, bool) {
	m, ok := Lookup(name)
	if !ok || len(m.Body) == 0 {
		return nil, false
	}
	return m.Body, true
}

// Op returns the operation name of a wire method — the name the door's
// refusal carries.
//
// It answers false for a method the contract does not know.
func Op(name string) (string, bool) {
	m, ok := Lookup(name)
	if !ok {
		return "", false
	}
	return m.Op, true
}

// CheckShape compares a method's params schema with the Go type the server
// reads it as: the same fields, with the same types, in both directions.
//
// A field the server reads but the schema does not name is a hole in the
// door; a field the schema names but the server does not read is a promise
// the server cannot keep. Both are reported, and the report names them.
//
// It answers an error for a method the contract does not know, and for a
// method that reads no params — a shape cannot be checked against nothing.
func CheckShape(name string, typ reflect.Type) error {
	shape, ok := ParamsSchema(name)
	if !ok {
		return fmt.Errorf("the contract names no params for the %q method", name)
	}
	return jsonshape.CheckAgainst(shape, typ)
}
