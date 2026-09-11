// SPDX-License-Identifier: MIT

package governor

// Governor exported error types: ErrNoModelConfigured + ErrNoProviders +
// their methods. Carved out of governor.go during the Day 29 god file
// split #2 so the main file can focus on constructor + ceiling +
// provider routing.

import (
	"fmt"
	"strings"
)

type ErrNoModelConfigured struct {
	TaskType string
}

func (e *ErrNoModelConfigured) Error() string {
	if e != nil && strings.TrimSpace(e.TaskType) != "" {
		return fmt.Sprintf("governor: no model configured for task %q — set AGEZT_MODEL, a per-task routing model, or a fallback chain", e.TaskType)
	}
	return "governor: no model configured — set AGEZT_MODEL, a per-task routing model, or a fallback chain"
}

// ErrNoProviders is returned when no provider in the chain succeeded.
type ErrNoProviders struct {
	Tried []string
	Last  error
}

func (e *ErrNoProviders) Error() string {
	return fmt.Sprintf("governor: all providers failed (tried %v): %v", e.Tried, e.Last)
}

func (e *ErrNoProviders) Unwrap() error { return e.Last }

// preflightAndRoute runs every pre-call gate (task/down-route model remap,
// capability + strict-pricing gates, rate-limit and budget pre-checks) against
// req — mutating it in place where a gate remaps the model — then resolves and
// announces the provider chain. Shared by Complete and CompleteStream so the
// governed call is byte-for-byte identical whether or not the response streams.
// Returns the routed chain, or a non-nil error if any gate refuses the call.
//
// Routing hints can be smuggled via the request: not exposed at the
// agent.Provider boundary in M1.b. The future Planner will pass options
// through a richer interface.
// preflightAndRoute runs the pre-dispatch cascade (see preflight.go) and, if
// nothing refuses the call, picks the provider chain to serve it.
