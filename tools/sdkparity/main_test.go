// SPDX-License-Identifier: MIT

package main

import "testing"

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
