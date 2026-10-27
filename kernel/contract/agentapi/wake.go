// SPDX-License-Identifier: MIT

package agentapi

import (
	"context"
	"strings"
)

// WakeContext is durable provenance for why a run exists. It is stamped on
// task.received by the agent loop and intentionally kept separate from the
// prompt.
//
// The row lives in the CONTRACT layer, because it is provenance about a RUN
// and not a mechanism of the runtime: the host that starts a run (the
// control plane's dispatcher, the roster's operator wake) has to state WHY
// the run exists, and the host must not have to name the runtime's module to
// do it. The runtime reads the row from here.
type WakeContext struct {
	Source            string
	Reason            string
	ScheduleID        string
	StandingID        string
	StandingName      string
	TriggerSubject    string
	ParentCorrelation string
}

// wakeContextKey is the context key the provenance travels under. It is
// unexported and unique to this package, so nothing outside can forge or
// shadow it.
type wakeContextKey struct{}

// WithWakeContext attaches run provenance to the next agent loop. Empty fields
// are omitted from the journal; callers can layer it with the agent profile.
func WithWakeContext(ctx context.Context, w WakeContext) context.Context {
	w.Source = strings.TrimSpace(w.Source)
	w.Reason = strings.TrimSpace(w.Reason)
	w.ScheduleID = strings.TrimSpace(w.ScheduleID)
	w.StandingID = strings.TrimSpace(w.StandingID)
	w.StandingName = strings.TrimSpace(w.StandingName)
	w.TriggerSubject = strings.TrimSpace(w.TriggerSubject)
	w.ParentCorrelation = strings.TrimSpace(w.ParentCorrelation)
	if w == (WakeContext{}) {
		return ctx
	}
	return context.WithValue(ctx, wakeContextKey{}, w)
}

// WakeContextFrom returns the run provenance attached to ctx. ok=false means
// the run carries none — the zero value is returned with it.
func WakeContextFrom(ctx context.Context) (WakeContext, bool) {
	if ctx == nil {
		return WakeContext{}, false
	}
	v, ok := ctx.Value(wakeContextKey{}).(WakeContext)
	return v, ok
}
