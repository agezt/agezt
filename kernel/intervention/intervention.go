// SPDX-License-Identifier: MIT

// Package intervention defines the protocol grammar for safe live changes to a
// running agent. Runtime owns execution; this package is only the wire contract.
package intervention

import (
	"errors"
	"strings"
	"time"
)

// Primitive is the operator's intended intervention semantics.
type Primitive string

const (
	PrimitiveHalt     Primitive = "halt"
	PrimitiveAbort    Primitive = "abort"
	PrimitiveRedirect Primitive = "redirect"
	PrimitiveAdjust   Primitive = "adjust"
	PrimitiveQuery    Primitive = "query"
)

// DefaultLease bounds interventions whose caller did not name a duration.
const DefaultLease = 5 * time.Minute

// Request is the normalized intervention command.
type Request struct {
	Primitive      Primitive
	CorrelationID  string
	Directive      string
	Lease          time.Duration
	Scope          string
	IdempotencyKey string
}

// Result is the transaction result returned to the caller and journaled.
type Result struct {
	Primitive      Primitive
	CorrelationID  string
	Accepted       bool
	Applied        bool
	State          string
	Paused         bool
	Pending        int
	LeaseExpires   time.Time
	IdempotencyKey string
	Reason         string
}

// Normalize validates and fills request defaults.
func Normalize(req Request) (Request, error) {
	req.CorrelationID = strings.TrimSpace(req.CorrelationID)
	req.Directive = strings.TrimSpace(req.Directive)
	req.Scope = strings.TrimSpace(req.Scope)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Primitive = Primitive(strings.ToLower(strings.TrimSpace(string(req.Primitive))))
	if req.CorrelationID == "" {
		return Request{}, errors.New("intervention: correlation required")
	}
	switch req.Primitive {
	case PrimitiveHalt, PrimitiveAbort, PrimitiveQuery:
	case PrimitiveRedirect, PrimitiveAdjust:
		if req.Directive == "" {
			return Request{}, errors.New("intervention: directive required")
		}
	default:
		return Request{}, errors.New("intervention: primitive must be halt|abort|redirect|adjust|query")
	}
	if req.Lease <= 0 {
		req.Lease = DefaultLease
	}
	if req.Scope == "" {
		req.Scope = "run"
	}
	return req, nil
}
