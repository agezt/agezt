// SPDX-License-Identifier: MIT

package restapi

// The daemon's request door.
//
// The `/api/v1` surface answers to the SDKs, and the SDKs build their bodies
// from the contract's request rows — a typed row, so a typed body. The daemon
// used to decode those bodies into structs of its own, which meant a mistyped
// body arrived as a silent default: `{"help": "yes"}` was not a help request,
// it was no help request at all, and `{"help": null}` was the same silence.
//
// The door closes that. A body that does not fit the CONTRACT's row is
// refused before the handler reads it, and the refusal carries the same words
// the operation registry's door gives:
//
//	operation <op> input: $.<field> has wrong type: got <got>, want <want>
//
// So one caller learns one language: a mistyped body is the same mistake
// whether it was sent to the registry or to the REST surface.
//
// Two rules keep the door the registry's door:
//
//  1. It checks TYPES, not presence. The schema carries no `required` list —
//     an absent intent is still the handler's "intent is required", not a
//     schema error.
//  2. It measures the CONTRACT's row, not the daemon's struct. The rows are
//     the one home for the request vocabulary (W5.10), so the door catches a
//     contract drift at the door instead of at the handler.
//
// What the door does NOT change: the handler still reads its own struct, the
// bytes on the wire did not move, and a body that fits the contract is
// decoded exactly as before.

import (
	"errors"
	"net/http"

	"github.com/agezt/agezt/kernel/contract/restapiapi"
	"github.com/agezt/agezt/kernel/httpserver"
)

// decodeRequestBody reads the request body, checks it against the contract's
// request row for endpointID, and decodes it into target.
//
// It answers false when the request is refused; the caller must return then.
// The refusal is written the way the surface writes one: a 400 with an
// `invalid_request` error — except for a body over the size limit, which is
// the surface's 413 (`request_too_large`).
//
// The CHECK is the transport's shared core (kernel/httpserver); this file is
// the REST surface's voice of it — the JSON error envelope the surface has
// always answered with.
func decodeRequestBody(w http.ResponseWriter, r *http.Request, endpointID string, target any) bool {
	var shape []byte
	if declared, ok := restapiapi.RequestSchema(endpointID); ok {
		shape = declared
	}
	err := httpserver.DecodeJSONBody(r, shape, endpointID, target)
	if err == nil {
		return true
	}
	if errors.Is(err, httpserver.ErrBodyTooLarge) {
		writeErr(w, http.StatusRequestEntityTooLarge, "request_too_large",
			"request body exceeds the size limit")
		return false
	}
	var refusal *httpserver.SchemaRefusal
	if errors.As(err, &refusal) {
		writeErr(w, http.StatusBadRequest, "invalid_request", refusal.Error())
		return false
	}
	writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
	return false
}
