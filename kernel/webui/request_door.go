// SPDX-License-Identifier: MIT

package webui

// The console's request door.
//
// The console's own endpoints — the login surface, the rollback surface, the
// file manager, the voice surface — answer the BROWSER, and their request
// bodies were read into whatever struct (or, for the file manager, whatever
// map) the handler happened to use. A mistyped body was therefore decided by
// Go's decoder, and a mistyped field in a map body was not decided at all:
//
//	POST /api/files/mkdir {"path": 5}          → the path became "" and the
//	                                              mutation was sent to the
//	                                              daemon anyway
//	POST /api/files/mkdir {"parents": "yes"}    → the flag became false
//
// The console's own endpoint list (kernel/contract/opapi/native.go) now names
// each body's shape, and the console checks an arriving body against it before
// the handler reads it — with the same validator and in the same words the
// operation registry's door uses:
//
//	operation files_mkdir input: $.path has wrong type: got number, want string
//
// Two rules keep the door the registry's door:
//
//  1. It checks TYPES, not presence. The schema carries no `required` list —
//     an absent path is still the handler's "from and to are required", not a
//     schema error.
//  2. It measures the CONTRACT's schema, not the handler's struct. The two are
//     held to the same fields by a ratchet, so a body the contract describes
//     and a body the handler reads cannot drift apart.
//
// The CHECK is the transport's shared core (kernel/httpserver); this file is
// the console's voice of it. The console answers a refusal in its JSON
// envelope, so the browser can show the sentence instead of a bare status
// line — the endpoints that answer plain text lose their message in the
// browser today, and a refusal the door writes does not have to.

import (
	"errors"
	"net/http"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/httpserver"
)

// The console's request bodies, one struct per endpoint that takes one.
//
// These are the console's OWN structs — the contract names the shape, and
// these structs read it. The ratchet in console_native_bodies_test.go holds
// the two to the same fields, in both directions.
type (
	// loginBody is the body of POST /api/login.
	loginBody struct {
		Password string `json:"password"`
	}
	// rollbackApplyBody is the body of POST /api/rollback/apply.
	rollbackApplyBody struct {
		ID string `json:"id"`
	}
	// fileMkdirBody is the body of POST /api/files/mkdir.
	fileMkdirBody struct {
		Path    string `json:"path"`
		Parents bool   `json:"parents"`
	}
	// fileRenameBody is the body of POST /api/files/rename.
	fileRenameBody struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	// fileDeleteBody is the body of POST /api/files/delete.
	fileDeleteBody struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	// ttsBody is the body of POST /api/tts.
	ttsBody struct {
		Text string `json:"text"`
	}
)

// decodeNativeBody reads the request body, checks it against the schema the
// console's own endpoint list names for name, and decodes it into target.
//
// It answers true when the body may be read. When the body does not fit the
// contract — or is over the route's cap — it writes the refusal itself and
// answers false.
//
// A body that is not JSON at all is NOT the door's business: the door answers
// the decoder's error, and the endpoint writes it in its own voice, as it
// always has.
func decodeNativeBody(w http.ResponseWriter, r *http.Request, name opapi.NativeRouteName, target any) (bool, error) {
	shape, _ := opapi.ConsoleNativeBodySchema(name)
	err := httpserver.DecodeJSONBody(r, shape, string(name), target)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, httpserver.ErrBodyTooLarge) {
		// The console answers an oversized body with a 400, the way its own
		// endpoints always have (the REST surface answers 413). The status is
		// the console's; the sentence is clearer than the decoder's.
		writeJSON(w, http.StatusBadRequest,
			map[string]any{"error": "request body exceeds the size limit"})
		return false, nil
	}
	var refusal *httpserver.SchemaRefusal
	if errors.As(err, &refusal) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": refusal.Error()})
		return false, nil
	}
	return false, err
}
