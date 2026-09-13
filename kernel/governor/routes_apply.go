// SPDX-License-Identifier: MIT

package governor

// Route application helpers: applyTaskRouteRequire + applyTaskRoute.
// Carved out of routes.go during the Day 196 god-file split so the
// main file can stay focused on types + parsers and the parsers file
// can stay focused on the env-string parsers.
// Public API unchanged.

import ()

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

