// SPDX-License-Identifier: MIT

// Package consoleapi is the contract the console's own answers agree on.
//
// The console (kernel/webui) has two kinds of answers. Its **proxied** ones
// are typed already: they come from the app layer through the operation
// registry, and the registry's rows are the contract. Its **own** ones — the
// answers the console writes itself, for the routes that are not operation
// bindings — had no home: each one was a `map[string]any` written by hand in
// the surface, so the field names lived in an implementation package and the
// browser read names nobody had declared.
//
// These rows are that declaration. They live here, in the console's own
// contract, for the same reason the REST surface's rows live in
// `kernel/contract/restapiapi`: a surface's own rows have no other home, and
// a row two packages name must be declared once.
//
// The rules of the road:
//
//   - Rows only. No handlers, no I/O, no policy.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
//
// The fields are declared in the order the wire speaks them, because the field
// order IS the member order of the JSON the console serves.
package consoleapi

import "encoding/json"

// AuthMeta answers the browser's first question: is there a password gate,
// and has this request already been let in?
//
// It leaks the EXISTENCE of a gate, never data — the browser needs it to know
// whether to render the login screen.
type AuthMeta struct {
	Authed           bool `json:"authed"`
	PasswordRequired bool `json:"password_required"`
}

// Login is the answer to a login attempt.
//
// PasswordRequired is a POINTER because it says two different things: absent
// means "the gate is there and you are through it", and present means "there
// is no gate at all". The pointer is how the row says *absent*.
type Login struct {
	OK               bool  `json:"ok"`
	PasswordRequired *bool `json:"password_required,omitempty"`
}

// SSEToken is the ephemeral token the browser's EventSource carries in its
// query string, so the console's bearer token never rides a URL.
type SSEToken struct {
	Token string `json:"token"`
}

// Logout is the answer to a logout: the session cookie is gone.
type Logout struct {
	OK bool `json:"ok"`
}

// VoiceStatus answers which server-side speech halves are wired — without
// invoking either provider and without exposing an endpoint, a model or a
// credential.
type VoiceStatus struct {
	STT VoiceHalf `json:"stt"`
	TTS VoiceHalf `json:"tts"`
}

// VoiceHalf is one speech half's readiness.
type VoiceHalf struct {
	Configured bool `json:"configured"`
}

// FileTree answers the file manager's listing: the directory it lists and the
// entries it found.
//
// The rows are the console's own — the file manager is a console-native
// endpoint, not an operation binding — so they live here with the rest of the
// console's answers.
type FileTree struct {
	Root  string     `json:"root"`
	Nodes []FileNode `json:"nodes"`
}

// FileNode is one entry in the tree.
//
// Type is a plain string on purpose: the daemon says "dir" or "file", but the
// row does not narrow it. A third kind of entry must not break the reader —
// and the row says what the daemon sends, not what the reader hopes for.
//
// Size and ModifiedMS are omitted when they are zero, so a directory carries
// no size and a file with no timestamp carries none.
type FileNode struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       int64  `json:"size,omitempty"`
	ModifiedMS int64  `json:"modified_ms,omitempty"`
}

// WorkflowHook is the answer to a webhook.
//
// The hook answers in one of two shapes: it either RAN the workflow and
// finished while the caller waited (the reply — it carries the outputs and
// says ok), or it ACCEPTED the run and will finish it in the background. One
// row serves both: the members the other shape does not carry are absent.
//
// The workflow's own members are the daemon's row, forwarded verbatim: this
// contract cannot name another operation's row, and the wire does not care
// how the daemon held them. CorrelationID and Workflow are POINTERS for the
// same reason as Login's — a pointer says *absent*, and the daemon's row
// omits them when it has nothing to say.
type WorkflowHook struct {
	Accepted      bool    `json:"accepted,omitempty"`
	CorrelationID *string `json:"correlation_id"`
	Executed      any     `json:"executed,omitempty"`
	OK            bool    `json:"ok,omitempty"`
	Outputs       any     `json:"outputs,omitempty"`
	Workflow      *string `json:"workflow"`
}

// StreamOpen is the first frame of a streamed answer: the stream is open and
// the events follow.
type StreamOpen struct {
	Kind string `json:"kind"`
}

// StreamEvent is one event of a streamed answer: what happened, to what, and
// with which payload.
type StreamEvent struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Subject string          `json:"subject"`
}

// RunStreamEvent is one event of the RUN stream: the run's own events also
// carry the correlation that ties them to the run they belong to.
type RunStreamEvent struct {
	CorrelationID string          `json:"correlation_id"`
	Kind          string          `json:"kind"`
	Payload       json.RawMessage `json:"payload"`
	Subject       string          `json:"subject"`
}

// StreamError is the last frame of a stream that failed.
type StreamError struct {
	Error string `json:"error"`
	Kind  string `json:"kind"`
}

// StreamResult is the last frame of a stream that finished: the operation's
// own answer, forwarded verbatim.
type StreamResult struct {
	Kind   string `json:"kind"`
	Result any    `json:"result"`
}
