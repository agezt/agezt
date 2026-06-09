// SPDX-License-Identifier: MIT

package governor

import (
	"fmt"
	"strconv"
	"strings"
)

// TaskModelOverrides maps a task-type hint to a model id that
// replaces CompletionRequest.Model when the task type matches
// (M1.ll). Lets operators pin cheap models for low-stakes work
// without rewriting every caller:
//
//	AGEZT_TASK_MODEL_OVERRIDES="salience=claude-haiku-4-5-20251001;plan=claude-opus-4-7"
//
// **Why an override, not a default.** Callers (planner, agent
// loop, etc.) already pass a model — usually the daemon's
// configured default. Overriding "from above" via Governor means
// the operator can switch the model used by every salience call
// without touching the salience code. Conversely, a caller that
// *needs* a specific model (a planner targeting JSON output that
// only works on a particular tier) can set the model directly
// and not declare a TaskType — the override only fires when
// TaskType is set.
//
// **Composition with TaskRoutes / TaskRouteRequires.** The model
// override is applied independently — it changes what model gets
// requested, while routing changes which provider gets the
// request. Both can be active simultaneously (cheap model on the
// cheap provider for embed tasks, for example).
type TaskModelOverrides map[string]string

// TaskRouteRequires maps a task-type hint to an ordered list of
// provider names that are the ONLY candidates the Governor will
// try (M1.kk). Unlike TaskRoutes, which hoists preferred providers
// to the front of the full chain (soft preference, falls through
// on failure), TaskRouteRequires hard-restricts the chain — when
// all listed providers fail, the call fails with ErrNoProviders
// rather than falling through to other registered providers.
//
// **When to use which.**
//
//   - TaskRoutes: "I prefer Claude for planning, but Gemini is
//     fine if Anthropic is down."
//   - TaskRouteRequires: "Compliance forbids embedding data being
//     sent to any provider other than the in-VPC ollama; failure
//     is preferable to leakage."
//
// The two compose: a TaskRouteRequires for a task type takes
// precedence over the TaskRoutes entry for the same type (the
// hard restriction wins). Unknown / unregistered required
// providers behave the same way as TaskRoutes: silently dropped
// from the chain. When ALL required providers are unregistered,
// the call fails immediately rather than falling through to the
// rest of the chain — that's the whole point of "require."
//
// Operators wire via AGEZT_TASK_ROUTE_REQUIRES (parsed by
// ParseTaskRoutesEnv — same syntax as AGEZT_TASK_ROUTES).
type TaskRouteRequires map[string][]string

// TaskRoutes maps a task-type hint (e.g. "plan", "code", "embed")
// to an ordered list of provider names that should be tried first
// for that task type. The first registered provider from the list
// is tried first; the rest of the chain follows in the standard
// subscription-first order, with the named providers removed (so
// they don't get tried twice).
//
// **Semantics — soft preference, not hard pinning.** If the named
// provider isn't registered, the entry is silently skipped; routing
// falls through to the default chain. This way an operator-supplied
// TaskRoute that names a provider the daemon couldn't load (e.g.
// missing credentials at startup) degrades gracefully — the task
// still runs against whoever is available — rather than failing
// closed.
//
// **Why a list, not a single name.** Operators commonly want a
// preference order ("plan tasks: prefer claude, then gpt-4o"). The
// chain falls through the listed providers in order before reaching
// the default ordering, so the operator can express both intent
// and fallback in one entry.
//
// **Why operator-config, not LLM-decided.** Task-type-aware
// routing is a budget/control concern (cheap model for salience,
// strong model for planning) that the operator owns. Letting the
// model self-select would re-introduce the "model picks its own
// upgrade path" footgun DECISIONS C2 is meant to prevent.
type TaskRoutes map[string][]string

// parseTaskRoutesEnv decodes a `AGEZT_TASK_ROUTES`-style spec into
// a TaskRoutes map.
//
//	plan=anthropic;code=anthropic,openai;embed=ollama
//
//	→ TaskRoutes{
//	      "plan":  {"anthropic"},
//	      "code":  {"anthropic", "openai"},
//	      "embed": {"ollama"},
//	  }
//
// Whitespace around tokens is trimmed. Empty entries are skipped.
// Duplicate task types: later wins (last-write).
//
// Returns an error only on hard syntax problems (an entry with no
// `=`); unknown provider names are tolerated and surface later as
// silent skips in chain construction (see TaskRoutes doc).
func parseTaskRoutesEnv(spec string) (TaskRoutes, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	out := TaskRoutes{}
	for entry := range strings.SplitSeq(spec, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key, val, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("governor: task-route entry %q missing '='", entry)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("governor: task-route entry %q has empty task type", entry)
		}
		var providers []string
		for p := range strings.SplitSeq(val, ",") {
			if p = strings.TrimSpace(p); p != "" {
				providers = append(providers, p)
			}
		}
		if len(providers) == 0 {
			// Empty list — treat as "remove any prior route for this key"
			// rather than register an empty route.
			delete(out, key)
			continue
		}
		out[key] = providers
	}
	return out, nil
}

// ParseTaskModelOverridesEnv decodes
//
//	"plan=claude-opus-4-7;salience=claude-haiku-4-5-20251001"
//
// into a TaskModelOverrides map. Each entry must be
// `<task-type>=<model-id>`. Whitespace tolerated; later wins on
// duplicate keys; empty value deletes a prior entry.
func ParseTaskModelOverridesEnv(spec string) (TaskModelOverrides, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	out := TaskModelOverrides{}
	for entry := range strings.SplitSeq(spec, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key, val, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("governor: task-model entry %q missing '='", entry)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			return nil, fmt.Errorf("governor: task-model entry %q has empty task type", entry)
		}
		if val == "" {
			delete(out, key)
			continue
		}
		out[key] = val
	}
	return out, nil
}

// ParseTaskRoutesEnv is the exported wrapper for parseTaskRoutesEnv.
// Used by the daemon's startup wiring to translate the
// `AGEZT_TASK_ROUTES` env var into a TaskRoutes the governor
// understands.
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
func applyTaskRouteRequire(chain []*ProviderInfo, requires TaskRouteRequires, taskType string) []*ProviderInfo {
	if taskType == "" || len(requires) == 0 {
		return chain
	}
	required, ok := requires[taskType]
	if !ok || len(required) == 0 {
		return chain
	}
	byName := make(map[string]*ProviderInfo, len(chain))
	for _, p := range chain {
		byName[p.Name] = p
	}
	out := make([]*ProviderInfo, 0, len(required))
	for _, name := range required {
		if info, found := byName[name]; found {
			out = append(out, info)
		}
	}
	if len(out) == 0 {
		// Sentinel: returning nil tells the Governor to surface
		// ErrNoProviders rather than walk the unrestricted chain.
		return nil
	}
	return out
}

// applyTaskRoute returns the chain reordered for the given task
// type, given the default chain (already in subscription-first
// order with fallbacks appended) and the configured routes.
//
// Behaviour:
//   - taskType empty OR no routes configured for it → returns chain unchanged.
//   - matching route lists providers in order; for each registered name
//     in the list, that provider is hoisted to the front of the chain
//     (preserving the list's order). Names not registered in the
//     current registry are silently skipped.
//   - Providers listed in the route are removed from their old
//     positions in chain (so we don't try them twice).
//   - The remainder of chain (in original order) follows the hoisted
//     providers — providing the natural "preference then default
//     fallback" semantics.
//
// Pure function on the inputs — no mutation of chain or routes;
// caller gets a fresh slice they can reorder freely without
// affecting future calls.
func applyTaskRoute(chain []*ProviderInfo, routes TaskRoutes, taskType string) []*ProviderInfo {
	if taskType == "" || len(routes) == 0 {
		return chain
	}
	preferred, ok := routes[taskType]
	if !ok || len(preferred) == 0 {
		return chain
	}
	// Index chain by name for O(1) lookup.
	byName := make(map[string]*ProviderInfo, len(chain))
	for _, p := range chain {
		byName[p.Name] = p
	}
	// Build the hoisted prefix in the route's listed order, skipping
	// names that aren't currently registered.
	hoisted := make([]*ProviderInfo, 0, len(preferred))
	hoistedSet := make(map[string]struct{}, len(preferred))
	for _, name := range preferred {
		if info, found := byName[name]; found {
			if _, dup := hoistedSet[name]; dup {
				continue
			}
			hoisted = append(hoisted, info)
			hoistedSet[name] = struct{}{}
		}
	}
	if len(hoisted) == 0 {
		// None of the preferred providers are registered — degrade
		// gracefully to default ordering rather than returning
		// nothing.
		return chain
	}
	// Append every other chain entry (in original order) after the
	// hoisted prefix.
	out := make([]*ProviderInfo, 0, len(chain))
	out = append(out, hoisted...)
	for _, p := range chain {
		if _, hoistedAlready := hoistedSet[p.Name]; hoistedAlready {
			continue
		}
		out = append(out, p)
	}
	return out
}
