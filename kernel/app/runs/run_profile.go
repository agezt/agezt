// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"fmt"
	"strings"

	appexecprofile "github.com/agezt/agezt/kernel/app/execprofile"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

// WardenProfile resolves a warden-family execution profile id (local, warden
// or container) and layers its sandbox override onto ctx, returning the
// effective label.
//
// It is the one place that decides whether a warden-family profile is
// routable and whether its backend is available, so the direct run path and
// the dispatched seat path cannot drift apart.
//
// It takes the EXECUTION PROFILE family's ports and not the handle they were
// filled from: the warden is that family's business — the guard says so by
// name — and a file that names the runtime is the port's business.
func WardenProfile(ctx context.Context, ports appexecprofile.Ports, id string) (context.Context, string, error) {
	p, ok := executionprofile.WardenProfileForRun(id)
	if !ok {
		return ctx, "", fmt.Errorf("execution profile %q is not routable for run tools yet (supported: %s)",
			id, strings.Join(executionprofile.RoutableRunProfileIDsFor(ports.Inventory()), ", "))
	}
	if p == warden.ProfileContainer && ports.EffectiveProfile(p) != warden.ProfileContainer {
		return ctx, "", fmt.Errorf("execution profile %q requires an active container backend (set AGEZT_WARDEN_DOCKER=1 and configure AGEZT_WARDEN_DOCKER_IMAGE/runtime)", id)
	}
	return warden.WithProfileOverride(ctx, p), string(p), nil
}
