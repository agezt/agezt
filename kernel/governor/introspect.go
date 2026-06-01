// SPDX-License-Identifier: MIT

package governor

// Read-only views of the configured routing tables (M108). The governor parses
// AGEZT_TASK_ROUTES / AGEZT_TASK_ROUTE_REQUIRES / AGEZT_TASK_MODEL_OVERRIDES at
// startup, but an operator had no way to confirm what actually loaded — a typo
// in the env silently degrades to the default chain. These accessors return
// independent copies so callers (the control plane's config handler) can surface
// the EFFECTIVE tables without reaching into governor internals or mutating them.

// TaskRoutesView returns a copy of the per-task-type routing preferences:
// task type → ordered provider preference list. Empty when none configured.
func (g *Governor) TaskRoutesView() map[string][]string {
	return copyStringSliceMap(g.cfg.TaskRoutes)
}

// TaskRouteRequiresView returns a copy of the per-task-type HARD route pins:
// task type → required provider list (restrictive, not preferential).
func (g *Governor) TaskRouteRequiresView() map[string][]string {
	return copyStringSliceMap(map[string][]string(g.cfg.TaskRouteRequires))
}

// TaskModelOverridesView returns a copy of the per-task-type model overrides:
// task type → model id substituted into the request.
func (g *Governor) TaskModelOverridesView() map[string]string {
	out := make(map[string]string, len(g.cfg.TaskModelOverrides))
	for k, v := range g.cfg.TaskModelOverrides {
		out[k] = v
	}
	return out
}

func copyStringSliceMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, v := range in {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
