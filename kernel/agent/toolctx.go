// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"path/filepath"
	"strings"
)

// corrKey is the unexported context key under which the running loop stashes the
// current run's correlation id, so a Tool's Invoke can attribute its own side
// effects (journaled mutations, posts, learned skills) to the run that caused
// them — without threading the id through every tool's input schema.
type corrKey struct{}
type policyToolDefKey struct{}
type untrustedObservationTaintKey struct{}

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

// WithPolicyToolDef carries the already-resolved ToolDef for the tool call
// currently being gated. It lets runtime policy inspect dynamic MCP/forge tool
// metadata without giving the model authority to mutate that metadata.
func WithPolicyToolDef(ctx context.Context, def ToolDef) context.Context {
	return context.WithValue(ctx, policyToolDefKey{}, def)
}

// PolicyToolDefFromContext returns the ToolDef attached by the agent loop
// before invoking the policy hook.
func PolicyToolDefFromContext(ctx context.Context) (ToolDef, bool) {
	if ctx == nil {
		return ToolDef{}, false
	}
	def, ok := ctx.Value(policyToolDefKey{}).(ToolDef)
	return def, ok
}

// UntrustedObservationTaint is carried from the tool-output boundary to the
// next policy decision. It lets policy see that a proposed action is downstream
// of external data without asking the LLM to self-report that dependency.
type UntrustedObservationTaint struct {
	Sources       []string
	DirectiveLike bool
	Matches       []string
}

// WithUntrustedObservationTaint attaches the current run's external-observation
// taint to a policy context. Empty taints leave ctx unchanged.
func WithUntrustedObservationTaint(ctx context.Context, t UntrustedObservationTaint) context.Context {
	if len(t.Sources) == 0 && len(t.Matches) == 0 && !t.DirectiveLike {
		return ctx
	}
	return context.WithValue(ctx, untrustedObservationTaintKey{}, t)
}

// UntrustedObservationTaintFromContext returns the taint attached to a policy
// context, if any.
func UntrustedObservationTaintFromContext(ctx context.Context) (UntrustedObservationTaint, bool) {
	if ctx == nil {
		return UntrustedObservationTaint{}, false
	}
	t, ok := ctx.Value(untrustedObservationTaintKey{}).(UntrustedObservationTaint)
	return t, ok
}

// agentKey carries the slug of the named roster agent a run executes AS (M851),
// so a tool's side effect can be attributed to the agent that caused it — "this
// memory was added by researcher" — not just to the run. Empty when a run is the
// daemon's default identity (no named agent); tools then fall back to a generic
// source ("operator"/"agent").
type agentKey struct{}

// WithAgent returns a child context tagged with the acting agent's slug. The
// runtime stamps it when a run executes AS a named roster agent, alongside the
// correlation id, so provenance-aware tools (memory, skills) can record who.
// An empty slug leaves the context unchanged.
func WithAgent(ctx context.Context, slug string) context.Context {
	if slug == "" {
		return ctx
	}
	return context.WithValue(ctx, agentKey{}, slug)
}

// AgentFromContext returns the acting agent's slug stashed by WithAgent, or ""
// when the run is the daemon's default identity (or in a direct unit test).
func AgentFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if s, ok := ctx.Value(agentKey{}).(string); ok {
		return s
	}
	return ""
}

// workdirKey carries the run's per-agent working directory (M792): a
// workspace-RELATIVE subdirectory the file and shell tools operate inside
// when a run executes AS a named agent whose profile names one.
type workdirKey struct{}

// WithWorkdir returns a child context carrying a workspace-relative workdir.
// Defense-in-depth: absolute paths and any form of `..` escape are refused
// here (the context stays unchanged) even though the roster profile already
// validates the same rule — a tool must never receive an escaping workdir.
func WithWorkdir(ctx context.Context, workdir string) context.Context {
	w := cleanRelWorkdir(workdir)
	if w == "" {
		return ctx
	}
	return context.WithValue(ctx, workdirKey{}, w)
}

// WorkdirFromContext returns the per-agent workdir set by WithWorkdir, or ""
// (operate at the workspace root) when the run carries none.
func WorkdirFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if w, ok := ctx.Value(workdirKey{}).(string); ok {
		return w
	}
	return ""
}

// cleanRelWorkdir normalises a workdir to forward slashes and rejects
// absolute or escaping shapes ("" on any violation).
func cleanRelWorkdir(w string) string {
	w = strings.TrimSpace(w)
	if w == "" {
		return ""
	}
	s := filepath.ToSlash(w)
	if filepath.IsAbs(w) || strings.HasPrefix(s, "/") ||
		s == ".." || strings.HasPrefix(s, "../") || strings.Contains(s, "/../") || strings.HasSuffix(s, "/..") {
		return ""
	}
	return s
}
