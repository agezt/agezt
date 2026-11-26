// SPDX-License-Identifier: MIT

package restapiapi

import "encoding/json"

// The rows the daemon's REST surface SERVES.
//
// The table in restapi.go is the surface's vocabulary: the wire rows the SDKs
// speak, with their docs, their optionality and the order the catalogs
// render them in. These are the rows the daemon actually writes — the bytes.
//
// The two are held to each other by the ratchet (servedrows_test.go): every
// field the vocabulary names must be on the row the daemon serves, under the
// same name and a type it allows, and the served row may carry MORE (the
// server may add a field to a response).
//
// The fields are declared in the order the wire speaks them (alphabetically),
// because the field order IS the member order of the served JSON — and the
// golden fixtures pin it.
//
// The rows the surface shares with another domain live in that domain's
// contract (the board's rows in boardapi, the artifact index's in remoteapi,
// the update surface's in updateapi). These are the surface's OWN rows: the
// daemon's health, its models, its readiness, its errors and its runs.

// Health is the answer to "is the daemon up?": its status, its version, the
// model it answers with by default and how many models it holds.
type Health struct {
	DefaultModel string `json:"default_model"`
	ModelCount   int    `json:"model_count"`
	Status       string `json:"status"`
	Version      string `json:"version"`
}

// Models is the answer to "which models does this tenant have?": the default
// one and the whole set.
type Models struct {
	Default string   `json:"default"`
	Models  []string `json:"models"`
}

// ProbeStatus is the answer to a readiness probe: whether the daemon can serve
// work, and why not when it cannot.
type ProbeStatus struct {
	Reason string `json:"reason,omitempty"`
	Status string `json:"status"`
}

// APIError is the error body of every non-2xx answer: the machine-readable
// kind and the message.
type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// RunResult is the answer to a run: the correlation id, the model, the
// status, the answer — and, when the run failed, the error.
//
// The optional fields are POINTERS on purpose: they are how the row says
// "absent". The successful answer carries an answer even when the model
// returned an empty string (the row names it as required), while the failed
// answer carries an error and no answer, and the stream's frames carry only
// the part they have.
type RunResult struct {
	Answer        *string `json:"answer,omitempty"`
	CorrelationID string  `json:"correlation_id"`
	Error         *string `json:"error,omitempty"`
	Model         *string `json:"model,omitempty"`
	Status        *string `json:"status,omitempty"`
}

// RunArc is the answer to "what did this run journal?": the correlation id,
// how many events, and the events themselves.
//
// The events are carried as the bytes the daemon already marshalled — the
// contract may not name the platform's event type, and the wire does not care
// how the daemon held them.
type RunArc struct {
	CorrelationID string          `json:"correlation_id"`
	Count         int             `json:"count"`
	Events        json.RawMessage `json:"events"`
}
