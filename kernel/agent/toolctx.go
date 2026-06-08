// SPDX-License-Identifier: MIT

package agent

import "context"

// corrKey is the unexported context key under which the running loop stashes the
// current run's correlation id, so a Tool's Invoke can attribute its own side
// effects (journaled mutations, posts, learned skills) to the run that caused
// them — without threading the id through every tool's input schema.
type corrKey struct{}

// WithCorrelation returns a child context carrying the run's correlation id. The
// agent loop wraps each tool invocation's context with this so tools that mutate
// kernel state (e.g. the skill tool authoring a procedure) can journal under the
// originating run. An empty id leaves the context unchanged.
func WithCorrelation(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, corrKey{}, id)
}

// CorrelationFromContext returns the run correlation id stashed by the loop, or
// "" if the context carries none (e.g. a direct tool unit test). Tools should
// treat "" as "no run to attribute to" and fall back to their own source tag.
func CorrelationFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(corrKey{}).(string); ok {
		return id
	}
	return ""
}
