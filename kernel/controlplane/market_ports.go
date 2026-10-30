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
// The host does not decide what the marketplace is: the COMPOSITION ROOT fills the
// scope and the APPLICATION fills the family's own ports (`kernel/app/market/ports.go`).
// What stays here is the binding — which kernel answers the marketplace family's
// questions.
//
// The handle is never named here, and it is never held here either: the
// adapter holds the SCOPE — the family's ports, filled by the composition
// root — and this file does not import the runtime to say so.
func requestMarketPorts(ctx context.Context) appmarket.Ports {
	return requestScope(ctx).MarketPorts
}
