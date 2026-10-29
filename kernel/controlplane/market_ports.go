// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"

	appmarket "github.com/agezt/agezt/kernel/app/market"
)

// The REQUEST's marketplace ports.
//
// The marketplace is the daemon's own library of packs, and every surface that
// reads or writes it does so on the REQUEST's path — so the routed kernel is
// the one that answers, and there is no daemon-level binding: a binding nobody
// calls is dead code.
//
// The host does not decide what the marketplace is: it hands over the kernel
// handle and the APPLICATION fills its own ports (`kernel/app/market/ports.go`).
// What stays here is the binding — which kernel answers the marketplace family's
// questions.
//
// The handle is never named here: this file does not import the runtime, so the
// adapter may hold the handle without naming the runtime package to hand it
// over.
func requestMarketPorts(ctx context.Context) appmarket.Ports {
	return appmarket.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
