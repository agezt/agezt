// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestPulseFourteenPrimaryOperationsSocketTenantDenialHasNoEffects(t *testing.T) {
	provider := mock.New()
	_, server, _, dir := startPair(t, provider)
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	port := &fakePulse{}
	observers := &fakePulseObservers{}
	server.SetPulse(port)
	server.SetPulseObservers(observers)
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdPulseSubscribe, controlplane.CmdPulseStatus, controlplane.CmdPulseAsks, controlplane.CmdPulseAskResolve, controlplane.CmdPulsePause, controlplane.CmdPulseResume, controlplane.CmdPulseBeat, controlplane.CmdPulseCadence, controlplane.CmdPulseDial, controlplane.CmdPulseQuiet, controlplane.CmdPulseFlush, controlplane.CmdPulseWatch, controlplane.CmdPulseProbe, controlplane.CmdPulseUnwatch} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "issue_key": "owned", "seconds": 30, "path": "owned", "min_pct": 20, "name": "owned", "command": "echo owned", "dial": "quiet", "hours": "22-7"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	port.mu.Lock()
	defer port.mu.Unlock()
	if port.paused || port.beats != 0 || port.cadence != 0 || port.dial != "" || port.quiet != "" || port.flushed != 0 || port.removed != "" || port.resolvedKey != "" || len(observers.probeArgv) != 0 || provider.CallCount() != 0 {
		t.Fatal(port, observers, provider.CallCount())
	}
}
