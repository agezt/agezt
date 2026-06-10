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
