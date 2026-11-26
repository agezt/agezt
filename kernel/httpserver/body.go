// SPDX-License-Identifier: MIT

package httpserver

// The request-body door's shared core.
//
// Every surface that accepts a JSON request body checks it against the shape
// its CONTRACT declares before the handler reads it. The check itself is the
// same everywhere — the same validator, the same words — so it lives here,
// once:
//
//   - the REST surface (kernel/restapi) measures a body against the request
//     row the REST contract names;
//   - the console (kernel/webui) measures a body against the schema the
//     console's own endpoint list names.
//
// What each surface keeps is its own VOICE: the core says WHAT is wrong (the
// field, the type it got, the type it wanted), and the surface says how it
// refuses (the REST surface answers a JSON error envelope, the console
// answers its own text).
//
// The refusal's WORDS are the operation registry's — "operation <op> input:
// $.<field> has wrong type: got <got>, want <want>" — and they come from
// kernel/platform/schema, so the registry and every door say one thing.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/agezt/agezt/kernel/platform/schema"
)

// ErrBodyTooLarge says the request body is over the route's cap. The caller
// answers it with its own 413 envelope.
var ErrBodyTooLarge = errors.New("request body exceeds the size limit")

// SchemaRefusal is a request body that does not fit the shape its endpoint
// declares.
//
// Its message is the registry's: "operation <op> input: $.<field> has wrong
// type: got <got>, want <want>". The caller may write it as it is, or wrap it
// in its own envelope.
type SchemaRefusal struct {
	// Op is the endpoint's operation name — the registry names an operation,
	// and a door names the operation its endpoint serves.
	Op string
	// Err is the schema's own failure ("$.field has wrong type: ...").
	Err error
}

func (e *SchemaRefusal) Error() string { return schema.InputError(e.Op, e.Err).Error() }

// Unwrap gives errors.Is and errors.As the schema failure underneath.
func (e *SchemaRefusal) Unwrap() error { return e.Err }

// DecodeJSONBody reads a request body, checks it against a JSON Schema and
// decodes it into target.
//
// It is the ONE place a request body becomes a value, so every surface's door
// reads the same bytes the same way:
//
//   - an empty schema means the endpoint declares no shape: the body is read
//     and decoded, and nothing is checked;
//   - the schema is checked against the FIRST JSON value in the body — the
//     value a decoder reads — so a body with trailing bytes after a valid
//     object behaves as it always did;
//   - a body over the route's cap answers ErrBodyTooLarge;
//   - a body that does not fit the schema answers *SchemaRefusal;
//   - anything else (an empty body, malformed JSON) answers the decoder's own
//     error.
//
// The caller writes the refusal in its own voice.
func DecodeJSONBody(r *http.Request, shape json.RawMessage, op string, target any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return ErrBodyTooLarge
		}
		return err
	}
	value, err := firstJSONValue(body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(shape)) > 0 {
		if err := schema.ValidateJSON(shape, value); err != nil {
			return &SchemaRefusal{Op: op, Err: err}
		}
	}
	if err := json.NewDecoder(bytes.NewReader(value)).Decode(target); err != nil {
		return err
	}
	return nil
}

// CheckJSONBody measures a JSON body against a schema and decodes it.
//
// It is the sibling of DecodeJSONBody for a surface that must SEE the raw
// bytes before the body is parsed — a channel webhook verifies a signature
// over the bytes first, and only then may the body be read.
//
// Two things make it different from DecodeJSONBody, and both are the surface's
// own behaviour rather than the door's:
//
//   - the whole body must be ONE JSON value. A body with trailing bytes after
//     a valid object is refused here, the way it always was on a surface that
//     decoded it strictly (json.Unmarshal);
//   - the body is measured BEFORE it is decoded, so a mistyped field is
//     refused with the registry's words and never with the decoder's.
//
// A body that does not fit the schema answers a *SchemaRefusal (the
// registry's words); a body that is not JSON at all answers the decoder's own
// error.
func CheckJSONBody(body []byte, shape json.RawMessage, op string, target any) error {
	// The decoder's own error for a body that is not one JSON value — the
	// surface's own words for it, not a shape refusal.
	var probe any
	if err := json.Unmarshal(body, &probe); err != nil {
		return err
	}
	if len(bytes.TrimSpace(shape)) > 0 {
		if err := schema.ValidateJSON(shape, body); err != nil {
			return &SchemaRefusal{Op: op, Err: err}
		}
	}
	return json.Unmarshal(body, target)
}

// RefuseJSONBody writes a request body's refusal in the caller's voice.
//
// A body that does not fit the shape answers the registry's words — the
// operation's sentence, as plain text — and a body that is not JSON at all
// answers notJSON, the caller's own words for it ("bad json").
//
// The status is the caller's too: this writes 400, the status a body refusal
// has everywhere.
func RefuseJSONBody(w http.ResponseWriter, err error, notJSON string) {
	var refusal *SchemaRefusal
	if errors.As(err, &refusal) {
		http.Error(w, refusal.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, notJSON, http.StatusBadRequest)
}

// firstJSONValue returns the bytes of the first JSON value in body — the value
// a decoder reads from it, and so the value the door must check.
//
// An empty body is an EOF error, which is the error a surface has always
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
