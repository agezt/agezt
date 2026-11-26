// SPDX-License-Identifier: MIT

package acp

// The ACP surface's params door.
//
// The surface is named in the contract (kernel/contract/acpserverapi): every
// method says what params it takes, and the params are a JSON Schema. This
// file is the surface's DOOR — the place where an arriving params object is
// measured against that schema BEFORE the handler reads it.
//
// The refusal carries the operation registry's words:
//
//	operation session_new input: $.cwd has wrong type: got number, want string
//
// and the surface keeps its own VOICE: a JSON-RPC error object with the
// invalid-params code, which is what an ACP client already reads.
//
// Two rules keep the door the registry's door:
//
//  1. It checks TYPES, not presence. A missing field is still the handler's
//     business, not a schema error.
//  2. It measures the CONTRACT's schema, not the handler's struct — the
//     contract is the one home for the request vocabulary.
//
// The check itself is the shared door core (kernel/httpserver) — the same
// validator, the same words, the same measurement every surface uses; only
// the voice is this surface's.

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/contract/acpserverapi"
	"github.com/agezt/agezt/kernel/httpserver"
	"github.com/agezt/agezt/kernel/platform/acpclient"
)

// decodeParams measures an arriving params object against the shape the
// contract names for the method, and decodes it into target.
//
// It answers a refusal when the params do not fit; the caller must return
// then.
//
// A method the contract does not name has no shape to measure: the params
// are decoded and nothing is checked. A method that carries no params at all
// is left to the handler — presence is the handler's business, not the door's.
func decodeParams(method string, params json.RawMessage, target any) *acpclient.RPCError {
	op, known := acpserverapi.Op(method)
	if !known {
		if len(bytes.TrimSpace(params)) == 0 {
			return nil
		}
		if err := json.Unmarshal(params, target); err != nil {
			return invalidParams(err)
		}
		return nil
	}
	if len(bytes.TrimSpace(params)) == 0 {
		return nil
	}
	shape, measured := acpserverapi.ParamsSchema(method)
	var err error
	if measured {
		err = httpserver.CheckJSONBody(params, shape, op, target)
	} else {
		err = json.Unmarshal(params, target)
	}
	if err == nil {
		return nil
	}
	var refusal *httpserver.SchemaRefusal
	if errors.As(err, &refusal) {
		return &acpclient.RPCError{Code: codeInvalidParams, Message: refusal.Error()}
	}
	return invalidParams(err)
}

// invalidParams answers a params object the decoder itself refused — one that
// is not JSON at all, for one — in the surface's own words.
func invalidParams(err error) *acpclient.RPCError {
	return &acpclient.RPCError{Code: codeInvalidParams, Message: "invalid params: " + err.Error()}
}
