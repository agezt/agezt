// SPDX-License-Identifier: MIT

package governor

// Task-route env parsers: ParseTaskRoutesEnv + ParseTaskModelChainsEnv
// + ParseFallbackChainsEnv + ParseTaskBudgetsEnv. Carved out of
// routes.go during the Day 196 god-file split so the main file
// can stay focused on the TaskModelOverrides/TaskRouteRequires/
// TaskRoutes types + the shared parseTaskRoutesEnv + the model-
// override parser, and the apply file can stay focused on the
// route-application functions.
// Public API unchanged.

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseTaskRoutesEnv(spec string) (TaskRoutes, error) { return parseTaskRoutesEnv(spec) }

// TaskModelChains maps a task-type hint to an ORDERED list of model ids to try
// in turn (M703): the primary model first, then each fallback model. Unlike
// TaskModelOverrides (a single model id) + provider fallback (same model on
// another provider), a chain falls back model→model — each model routes to the
// provider that serves it (via applyModelRoute), so a failure of the whole
// primary-model attempt moves to the NEXT MODEL. This is true model-level
// fallback ("different fallback models per task").
//
//	AGEZT_TASK_MODEL_CHAINS="chat=claude-opus-4-7,gpt-5,deepseek-chat;code=gpt-5,claude-opus-4-7"
//
// A chain SUPERSEDES TaskModelOverrides for the same task type (the chain is the
// authoritative model selection). Same syntax/semantics as AGEZT_TASK_ROUTES:
// whitespace trimmed, empty entries skipped, later wins on duplicate keys, an
// empty value deletes a prior entry.
type TaskModelChains map[string][]string

// ParseTaskModelChainsEnv decodes a `AGEZT_TASK_MODEL_CHAINS` spec into a
// TaskModelChains map. The list values are MODEL ids (not provider names).
func ParseTaskModelChainsEnv(spec string) (TaskModelChains, error) {
	routes, err := parseTaskRoutesEnv(spec)
	if err != nil {
		// Re-label the error so it reads about models, not routes.
		return nil, fmt.Errorf("governor: task-model-chain: %w", err)
	}
	if routes == nil {
		return nil, nil
	}
	return TaskModelChains(routes), nil
}

// ParseFallbackChainsEnv decodes an `AGEZT_FALLBACK_CHAINS` spec into the named
// reusable chain registry (M963). Same syntax as AGEZT_TASK_MODEL_CHAINS — the
// key is the chain NAME (not a task type) and the value is its ordered model
// list: "fast=haiku,gpt-4o-mini;thorough=opus,gpt-5".
func ParseFallbackChainsEnv(spec string) (map[string][]string, error) {
	routes, err := parseTaskRoutesEnv(spec)
	if err != nil {
		return nil, fmt.Errorf("governor: fallback-chain: %w", err)
	}
	if routes == nil {
		return nil, nil
	}
	return map[string][]string(routes), nil
}

// ParseTaskBudgetsEnv decodes
//
//	"plan=100000;code=500000"
//
// into a map of task type → daily ceiling microcents (M1.zz).
// Each entry must be `<task-type>=<positive integer microcents>`.
// Zero or negative values are rejected at parse time so a typo
// doesn't silently disable the cap. Whitespace tolerated; later
// wins on duplicate keys.
//
// **Why microcents and not dollars.** Agezt's budget accounting
// is integer-microcents end-to-end (DECISIONS C1); accepting
// human-friendly dollars at the env-var boundary would create a
// rounding seam right where the operator's intent meets the
// enforcement layer. A wrapper script or the future `agt budget`
// command can do dollar→microcents conversion if needed.
func ParseTaskBudgetsEnv(spec string) (map[string]int64, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	out := map[string]int64{}
	for entry := range strings.SplitSeq(spec, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key, val, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("governor: task-budget entry %q missing '='", entry)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			return nil, fmt.Errorf("governor: task-budget entry %q has empty task type", entry)
		}
		if val == "" {
			return nil, fmt.Errorf("governor: task-budget entry %q has empty value (use a positive integer microcents)", entry)
		}
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("governor: task-budget entry %q: parse microcents: %w", entry, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("governor: task-budget entry %q: value must be > 0 (got %d)", entry, n)
		}
		out[key] = n
	}
	return out, nil
}

// applyTaskRouteRequire returns the chain restricted to the
// providers named in the route for the given task type, in
// listed order. Behaves like applyTaskRoute when no requires
// entry exists for taskType (returns chain unchanged).
//
// When a requires entry exists but no listed providers are
// registered, returns a SINGLE-element chain containing nil so
// the Governor can detect this case and fail closed — falling
// through to default ordering would defeat the "hard pin"
// guarantee operators rely on.
