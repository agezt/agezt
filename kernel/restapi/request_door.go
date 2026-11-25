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
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/agezt/agezt/kernel/contract/restapiapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// decodeRequestBody reads the request body, checks it against the contract's
// request row for endpointID, and decodes it into target.
//
// It answers false when the request is refused; the caller must return then.
// The refusal is written the way the surface writes one: a 400 with an
// `invalid_request` error — except for a body over the size limit, which is
// the surface's 413 (`request_too_large`).
func decodeRequestBody(w http.ResponseWriter, r *http.Request, endpointID string, target any) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request_too_large",
				"request body exceeds the size limit")
			return false
		}
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return false
	}
	// The handler's decoder reads the FIRST value in the body, so the door
	// checks that value — not a stricter one. A body with trailing bytes
	// after a valid object keeps behaving as it always did.
	value, err := firstJSONValue(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return false
	}
	if shape, ok := restapiapi.RequestSchema(endpointID); ok {
		if err := schema.ValidateJSON(shape, value); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request",
				"operation "+endpointID+" input: "+err.Error())
			return false
		}
	}
	if err := json.NewDecoder(bytes.NewReader(value)).Decode(target); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// firstJSONValue returns the bytes of the first JSON value in body — the value
// a decoder reads from it, and so the value the door must check.
//
// An empty body is an EOF error, which is the error the surface has always
// answered an empty body with.
func firstJSONValue(body []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var probe any
	if err := decoder.Decode(&probe); err != nil {
		return nil, err
	}
	rest, err := io.ReadAll(decoder.Buffered())
	if err != nil {
		return nil, err
	}
	return body[:len(body)-len(rest)], nil
}
