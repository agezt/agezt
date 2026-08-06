// SPDX-License-Identifier: MIT

package controlplane

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// Registry invariants: the command registry IS the dispatch surface (handleConn
// routes every request through it), so these tests pin it against protocol.go
// and enforce the tenant-isolation invariant. The tenant allowlist itself is
// swept end-to-end by tenant_auth_test.go's registry-driven exhaustive test.

// cmdConstRe matches the Cmd* string constants in protocol.go, e.g.
//
//	CmdVersion       = "version"
var cmdConstRe = regexp.MustCompile(`(?m)^\s*(Cmd\w+)\s*=\s*"([^"]+)"`)

// protocolCommands returns constName→value for every Cmd* const in protocol.go.
func protocolCommands(t *testing.T) map[string]string {
	t.Helper()
	src, err := os.ReadFile("protocol.go")
	if err != nil {
		t.Fatalf("read protocol.go: %v", err)
	}
	out := map[string]string{}
	for _, m := range cmdConstRe.FindAllStringSubmatch(string(src), -1) {
		if prev, dup := out[m[1]]; dup {
			t.Fatalf("protocol.go: const %s defined twice (%q, %q)", m[1], prev, m[2])
		}
		out[m[1]] = m[2]
	}
	if len(out) == 0 {
		t.Fatal("protocol.go: no Cmd* constants matched — regexp or file layout changed")
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// TestRegistry_MatchesProtocolConstants asserts set equality between the Cmd*
// constants declared in protocol.go and the commands in commandRegistry, in
// both directions. A new protocol command MUST be registered (add a
// commandSpec to the owning subsystem's registerXCommands), and the registry
// must never carry a command the protocol no longer declares.
func TestRegistry_MatchesProtocolConstants(t *testing.T) {
	consts := protocolCommands(t)
	declared := map[string]string{} // value → const name
	for name, val := range consts {
		if prev, dup := declared[val]; dup {
			t.Errorf("protocol.go: command string %q declared by both %s and %s", val, prev, name)
		}
		declared[val] = name
	}
	for _, val := range sortedKeys(declared) {
		if _, ok := commandRegistry[val]; !ok {
			t.Errorf("protocol.go declares %s = %q but it is not registered — add a commandSpec for it in the owning subsystem's registerXCommands()", declared[val], val)
		}
	}
	for _, cmd := range sortedKeys(commandRegistry) {
		if _, ok := declared[cmd]; !ok {
			t.Errorf("registry contains %q which no Cmd* constant in protocol.go declares — remove the stale commandSpec", cmd)
		}
	}
	if len(declared) != len(commandRegistry) {
		t.Errorf("protocol.go declares %d commands, registry has %d", len(declared), len(commandRegistry))
	}
}

// TestRegistry_TenantAllowedImpliesTenantRouted is PERMANENT: it enforces the
// real tenant-isolation invariant. Every command a tenant token may invoke
// must route to the caller's kernel (kernelFor / edictFor / projectJournal);
// a TenantAllowed handler that reads s.k directly would leak the PRIMARY
// kernel's data to a tenant.
//
// Sole exemption: whoami. handleWhoami only echoes the caller's identity
// (primary/tenant + the pinned tenant id from req.Args) and never touches any
// kernel, so there is nothing to route.
func TestRegistry_TenantAllowedImpliesTenantRouted(t *testing.T) {
	for _, cmd := range sortedKeys(commandRegistry) {
		spec := commandRegistry[cmd]
		if !spec.TenantAllowed || spec.TenantRouted {
			continue
		}
		if cmd == CmdWhoami {
			continue // identity echo — touches no kernel (see doc comment)
		}
		t.Errorf("%q is TenantAllowed but not TenantRouted — a tenant token would read the primary kernel's data; route the handler via kernelFor(tenantOf(req)) (or edictFor/projectJournal) and set TenantRouted", cmd)
	}
}
