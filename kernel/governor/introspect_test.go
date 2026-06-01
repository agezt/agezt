// SPDX-License-Identifier: MIT

package governor

import "testing"

func TestRoutingViews_CopyAndContent(t *testing.T) {
	g := &Governor{cfg: Config{
		TaskRoutes:         TaskRoutes{"plan": {"anthropic", "openai"}},
		TaskRouteRequires:  TaskRouteRequires{"secure": {"anthropic"}},
		TaskModelOverrides: TaskModelOverrides{"code": "claude-sonnet-4-6"},
	}}

	routes := g.TaskRoutesView()
	if got := routes["plan"]; len(got) != 2 || got[0] != "anthropic" || got[1] != "openai" {
		t.Fatalf("routes[plan] = %v", got)
	}
	// Mutating the view must not affect the governor's config.
	routes["plan"][0] = "TAMPERED"
	if g.cfg.TaskRoutes["plan"][0] != "anthropic" {
		t.Errorf("view mutation leaked into governor config")
	}

	if r := g.TaskRouteRequiresView(); r["secure"][0] != "anthropic" {
		t.Errorf("requires view = %v", r)
	}
	if o := g.TaskModelOverridesView(); o["code"] != "claude-sonnet-4-6" {
		t.Errorf("overrides view = %v", o)
	}

	// Empty governor → empty views, never nil-panic.
	empty := &Governor{cfg: Config{}}
	if len(empty.TaskRoutesView()) != 0 || len(empty.TaskModelOverridesView()) != 0 {
		t.Errorf("empty governor should yield empty views")
	}
}
