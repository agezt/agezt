// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestPulseFourteenPrimaryOperationsSocketTenantDenialHasNoEffects(t *testing.T) {
	provider := mock.New()
	port := &fakePulse{}
	observers := &fakePulseObservers{}
	_, _, _, dir, registry := startPairWithTenantsAndDeps(t, provider, func(string) controlplane.Deps {
		return controlplane.Deps{
			Pulse:     func() controlplane.PulseController { return port },
			Observers: func() controlplane.PulseObservers { return observers },
		}
	})
	token := mustTenant(t, registry, "acme")
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{opapi.PulseSubscribe, opapi.PulseStatus, opapi.PulseAsks, opapi.PulseAskResolve, opapi.PulsePause, opapi.PulseResume, opapi.PulseBeat, opapi.PulseCadence, opapi.PulseDial, opapi.PulseQuiet, opapi.PulseFlush, opapi.PulseWatch, opapi.PulseProbe, opapi.PulseUnwatch} {
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
