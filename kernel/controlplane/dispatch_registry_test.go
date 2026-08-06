// SPDX-License-Identifier: MIT

package controlplane

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// Phase 2.3 commit-A bridge tests: the command registry must be a PERFECT
// mirror of the legacy dispatch surface before commit B swaps handleConn onto
// it. Two of these tests pin the registry against legacy code that commit B
// deletes (and are deleted with it); the tenant-isolation invariant test is
// permanent.

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

// TestRegistry_MatchesOldSwitch asserts set equality between the `case Cmd*:`
// arms of handleConn's dispatch switch in server.go and commandRegistry.
// DELETE in commit B (the switch itself is deleted then).
func TestRegistry_MatchesOldSwitch(t *testing.T) {
	consts := protocolCommands(t)
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	caseRe := regexp.MustCompile(`(?m)^\s*case (Cmd\w+):`)
	inSwitch := map[string]string{} // value → const name
	for _, m := range caseRe.FindAllStringSubmatch(string(src), -1) {
		val, ok := consts[m[1]]
		if !ok {
			t.Fatalf("server.go: case %s: not a Cmd* constant declared in protocol.go", m[1])
		}
		if prev, dup := inSwitch[val]; dup {
			t.Errorf("server.go: duplicate case for %q (%s and %s)", val, prev, m[1])
		}
		inSwitch[val] = m[1]
	}
	if len(inSwitch) == 0 {
		t.Fatal("server.go: no `case Cmd*:` arms matched — regexp or file layout changed")
	}
	for _, val := range sortedKeys(inSwitch) {
		if _, ok := commandRegistry[val]; !ok {
			t.Errorf("handleConn switch dispatches %s (%q) but the registry does not — add a commandSpec", inSwitch[val], val)
		}
	}
	for _, cmd := range sortedKeys(commandRegistry) {
		if _, ok := inSwitch[cmd]; !ok {
			t.Errorf("registry contains %q which handleConn's switch does not dispatch — the registry must mirror the switch until commit B", cmd)
		}
	}
}

// TestRegistry_TenantAllowedMatchesLegacyAllowlist asserts that every spec's
// TenantAllowed flag equals tenantTokenAllows(cmd) — the registry copies the
// legacy allowlist verbatim. DELETE in commit B (tenantTokenAllows is deleted
// then; tenant_auth_test gains a registry-driven exhaustive table instead).
func TestRegistry_TenantAllowedMatchesLegacyAllowlist(t *testing.T) {
	for _, cmd := range sortedKeys(commandRegistry) {
		spec := commandRegistry[cmd]
		if got, want := spec.TenantAllowed, tenantTokenAllows(cmd); got != want {
			t.Errorf("%q: spec.TenantAllowed=%v but tenantTokenAllows=%v — the registry must mirror tenant.go's allowlist verbatim", cmd, got, want)
		}
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
