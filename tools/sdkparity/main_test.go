// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestExtractRoutes_FindsLiveRoutes is the guard whose absence let sdkparity
// rot silently. `extractRoutes` greps the daemon's route table by call shape;
// when c7dded37 moved registrations from `mux.HandleFunc(path, h)` to
// `router.Handle(path, policy, h)` the old pattern matched NOTHING, so the
// generated report listed zero routes and `-check` declared the (correct)
// docs/SDK-PARITY.md stale. Worse, the remedy it printed — regenerate with
// `-out` — would have deleted the real route table and rewritten every SDK's
// coverage to 0/0.
//
// Asserting a non-empty extraction against the REAL restapi.go turns that
// silent regression into a red test. The named-route check makes it a real
// assertion rather than a pulse: a pattern that matched some unrelated literal
// would still be caught.
func TestExtractRoutes_FindsLiveRoutes(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("runtime.Caller unavailable")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	routes, err := extractRoutes(filepath.Join(repoRoot, "kernel", "restapi", "restapi.go"))
	if err != nil {
		t.Fatalf("extractRoutes: %v", err)
	}
	if len(routes) == 0 {
		t.Fatal("extractRoutes found no /api/v1 routes in kernel/restapi/restapi.go — the route registration shape changed and the pattern no longer matches it; fix routeRegistration rather than regenerating the report, which would erase a correct table")
	}

	// Anchor on routes that must exist for the daemon to be a daemon. If the
	// pattern ever matches something else entirely, len() alone would pass.
	for _, want := range []string{"/api/v1/health", "/api/v1/runs"} {
		found := false
		for _, r := range routes {
			if r.Path == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("extractRoutes did not find %s; got %v", want, routes)
		}
	}
}

// TestExtractRoutes_MatchesBothRegistrationShapes pins the tolerance the
// pattern was widened to. Both the historical `mux.HandleFunc` form and the
// current `router.Handle` (policy-carrying) form must be recognised, and
// non-/api/v1 routes must stay out of the SDK report.
func TestExtractRoutes_MatchesBothRegistrationShapes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "restapi.go")
	if err := os.WriteFile(src, []byte(`package restapi
func (s *Server) Handler() http.Handler {
	mux.HandleFunc("/api/v1/legacy", s.handleLegacy)
	router.Handle("/api/v1/health", userRoute, s.handleHealth)
	router.Handle("/api/v1/runs/", userRoute, s.handleRunByID)
	router.Handle("/healthz", publicRoute, s.handleLive)
}
`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	routes, err := extractRoutes(src)
	if err != nil {
		t.Fatalf("extractRoutes: %v", err)
	}
	got := map[string]bool{}
	for _, r := range routes {
		got[r.Path] = true
		if r.Path == "/api/v1/runs/" && !r.Dynamic {
			t.Error("/api/v1/runs/ should be marked Dynamic (trailing slash = prefix route)")
		}
	}
	for _, want := range []string{"/api/v1/legacy", "/api/v1/health", "/api/v1/runs/"} {
		if !got[want] {
			t.Errorf("missing %s; got %v", want, routes)
		}
	}
	if got["/healthz"] {
		t.Error("/healthz is not an /api/v1 route and must not reach the SDK report")
	}
}

func TestRoutesForSDKCoverage_ExcludesOperationalUpdateRoutes(t *testing.T) {
	routes := []route{
		{Path: "/api/v1/health"},
		{Path: "/api/v1/update"},
		{Path: "/api/v1/update/apply"},
		{Path: "/api/v1/runs"},
	}
	got := routesForSDKCoverage(routes)
	if len(got) != 2 {
		t.Fatalf("len(routesForSDKCoverage) = %d, want 2: %#v", len(got), got)
	}
	for _, r := range got {
		if intentionallyUnsupportedInSDK(r.Path) {
			t.Fatalf("unsupported route %q included in SDK coverage routes", r.Path)
		}
	}
}
