// SPDX-License-Identifier: MIT

package openaiapi

// The OpenAI-compatible surface's request door.
//
// The surface's shape is named in the contract (kernel/contract/openaiapi):
// every route says what body it takes, and the body is a JSON Schema. This
// file is the surface's DOOR — the place where an arriving body is measured
// against that schema BEFORE the handler reads it.
//
// The refusal carries the operation registry's words:
//
//	operation <route> input: $.<field> has wrong type: got <got>, want <want>
//
// and the surface keeps its own VOICE: the OpenAI error envelope every
// OpenAI client already reads —
//
//	{"error":{"message":"operation chat_completions input: $.model has wrong type: got number, want string","type":"invalid_request_error"}}
//
// Two rules keep the door the registry's door:
//
//  1. It checks TYPES, not presence. A missing field is still the handler's
//     business (an absent model is still the default model).
//  2. It measures the CONTRACT's schema, not the handler's struct — the
//     contract is the one home for the request vocabulary, and a ratchet
//     holds the two to the same fields with the same types.
//
// The check itself is the transport's shared core (kernel/httpserver); this
// file is the OpenAI surface's voice of it.

import (
	"errors"
	"net/http"

	oaiapi "github.com/agezt/agezt/kernel/contract/openaiapi"
	"github.com/agezt/agezt/kernel/httpserver"
)

// decodeOpenAIBody reads the request body, checks it against the contract's
// schema for routeID, and decodes it into target.
//
// It answers false when the request is refused; the caller must return then.
// The refusal is written the way the OpenAI surface writes one: a 400 with
// the invalid_request_error type, the envelope every OpenAI client reads.
//
// A body that is not JSON at all is still the decoder's error — the surface
// has always answered "invalid JSON body: …" for it, and it does so still.
func decodeOpenAIBody(w http.ResponseWriter, r *http.Request, routeID string, target any) bool {
	// A route the contract does not name has no shape to measure: the body is
	// read and decoded, and nothing is checked.
	shape, _ := oaiapi.RequestSchema(routeID)
	err := httpserver.DecodeJSONBody(r, shape, routeID, target)
	if err == nil {
		return true
	}
	if errors.Is(err, httpserver.ErrBodyTooLarge) {
		writeErr(w, http.StatusRequestEntityTooLarge, "invalid_request_error",
			"request body exceeds the size limit")
		return false
	}
	var refusal *httpserver.SchemaRefusal
	if errors.As(err, &refusal) {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", refusal.Error())
		return false
	}
	writeErr(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body: "+err.Error())
	return false
}
