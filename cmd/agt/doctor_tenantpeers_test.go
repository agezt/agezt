// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestCheckTenantPeers_UnsetIsQuiet(t *testing.T) {
	if _, show := checkTenantPeers(""); show {
		t.Error("unset AGEZT_TENANT_PEERS should produce no check line")
	}
}

func TestCheckTenantPeers_EmptyObjectOK(t *testing.T) {
	c, show := checkTenantPeers("{}")
	if !show || c.Status != statusOK {
		t.Fatalf("empty object should be OK, got show=%v status=%v", show, c.Status.label())
	}
}

func TestCheckTenantPeers_Valid(t *testing.T) {
	spec := `{"alpha":"a=http://h1|t1,b=http://h2|t2","beta":"c=http://h3|t3"}`
	c, show := checkTenantPeers(spec)
	if !show || c.Status != statusOK {
		t.Fatalf("valid spec should be OK, got show=%v status=%s: %s", show, c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "2 tenant override(s)") {
		t.Errorf("detail = %q, want a 2-tenant count", c.Detail)
	}
	// Sorted, with per-tenant peer counts.
	if !strings.Contains(c.Detail, "alpha→2 peer(s)") || !strings.Contains(c.Detail, "beta→1 peer(s)") {
		t.Errorf("detail = %q, want per-tenant peer counts", c.Detail)
	}
	// Never leak a token or URL.
	if strings.Contains(c.Detail, "http://") || strings.Contains(c.Detail, "t1") {
		t.Errorf("detail leaked a URL/token: %q", c.Detail)
	}
}

func TestCheckTenantPeers_MalformedFails(t *testing.T) {
	cases := map[string]string{
		"not json":        `{not json`,
		"bad inner peers": `{"alpha":"missing-equals"}`,
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			c, show := checkTenantPeers(spec)
			if !show || c.Status != statusFail {
				t.Fatalf("expected FAIL, got show=%v status=%s", show, c.Status.label())
			}
			if c.Hint == "" {
				t.Error("a FAIL should carry a fix hint")
			}
		})
	}
}

func TestCheckTenantPeers_SilentlyDroppedTenantWarns(t *testing.T) {
	// A tenant with an empty peer set is silently dropped by the parser (and the
	// daemon) — doctor should surface that it was ignored.
	c, show := checkTenantPeers(`{"alpha":"a=http://h|t","ghost":""}`)
	if !show || c.Status != statusWarn {
		t.Fatalf("a dropped override should WARN, got show=%v status=%s: %s", show, c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "ghost") || !strings.Contains(c.Detail, "ignored") {
		t.Errorf("detail = %q, want it to name the ignored tenant", c.Detail)
	}
	// The loaded tenant is still reported.
	if !strings.Contains(c.Detail, "alpha→1 peer(s)") {
		t.Errorf("detail = %q, want the loaded tenant shown too", c.Detail)
	}
}

func TestCheckTenantPeers_AllDroppedWarns(t *testing.T) {
	// Every tenant empty → parser returns nothing, but the spec was non-empty,
	// so the operator clearly meant to configure something. WARN, not silent OK.
	c, show := checkTenantPeers(`{"ghost":""}`)
	if !show || c.Status != statusWarn {
		t.Fatalf("all-dropped should WARN, got show=%v status=%s: %s", show, c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "ghost") {
		t.Errorf("detail = %q, want it to name ghost", c.Detail)
	}
}
