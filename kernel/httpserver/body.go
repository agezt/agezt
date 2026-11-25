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
