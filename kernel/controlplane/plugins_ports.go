// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appplugins "github.com/agezt/agezt/kernel/app/plugins"
)

// The REQUEST's plugin ports.
//
// The plugin manifest is the external-plugin list the daemon was given at Open,
// and every surface that reads it — the plugin list and the config reader's
// plugin count — reads it on the REQUEST's path. So the routed kernel is the
// one that answers, and there is no daemon-level binding: a binding nobody
// calls is dead code, and the dead-code gate says so.
//
// The host does not decide what the manifest is: it hands over the kernel handle
// and the APPLICATION fills its own ports (`kernel/app/plugins/ports.go`). What
// stays here is the binding — which kernel answers the plugins family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func requestPluginPorts(ctx context.Context) appplugins.Ports {
	return appplugins.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
