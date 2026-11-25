// SPDX-License-Identifier: MIT

package agentgw

// The gateway's request door.
//
// The gateway's surface is named in the contract (kernel/contract/agentgwapi):
// every route says what body it takes, and the body is a JSON Schema. This
// file is the gateway's DOOR — the place where an arriving body is measured
// against that schema BEFORE the handler reads it.
//
// The refusal carries the operation registry's words:
//
//	operation <route> input: $.<field> has wrong type: got <got>, want <want>
//
// and the gateway keeps its own VOICE: the error envelope the subprocesses
// already read — {"error":{"code":"INVALID_REQUEST","message":"…"}}.
//
// Two rules keep the door the registry's door:
//
//  1. It checks TYPES, not presence. A missing field is still the handler's
//     "key is required", not a schema error.
//  2. It measures the CONTRACT's schema, not the handler's struct — the
//     contract is the one home for the request vocabulary.

import (
	"errors"
	"net/http"

	"github.com/agezt/agezt/kernel/contract/agentgwapi"
	"github.com/agezt/agezt/kernel/httpserver"
)

// decodeGatewayBody reads the request body, checks it against the contract's
// schema for routeID, and decodes it into target.
//
// It answers false when the request is refused; the caller must return then.
// The refusal is written the way the gateway writes one: a 400 with the
// INVALID_REQUEST code.
//
// The check itself is the transport's shared core (kernel/httpserver); this
// file is the gateway's voice of it.
func decodeGatewayBody(w http.ResponseWriter, r *http.Request, routeID string, target any) bool {
	// A route the contract does not name has no shape to measure: the body is
	// read and decoded, and nothing is checked.
	shape, _ := agentgwapi.RequestSchema(routeID)
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	err := httpserver.DecodeJSONBody(r, shape, routeID, target)
	if err == nil {
		return true
	}
	if errors.Is(err, httpserver.ErrBodyTooLarge) {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", "http: request body too large")
		return false
	}
	var refusal *httpserver.SchemaRefusal
	if errors.As(err, &refusal) {
		responseError(w, http.StatusBadRequest, "INVALID_REQUEST", refusal.Error())
		return false
	}
	responseError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	return false
}
