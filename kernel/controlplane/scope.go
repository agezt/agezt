// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/app/binding"
)

// scoped returns the SERVER's scope: the families' ports of the daemon the
// server serves.
//
// The server holds no handle, so this is the only way into the daemon's
// capabilities — and it is a scope, not the handle behind it.
func (s *Server) scoped() binding.Scope {
	return scopeOrRefusing(s.scope)
}

// requestScope returns the SCOPE of the request in flight: the daemon's, or
// the tenant's when the request is routed to a tenant.
//
// The request carries the scope, not the handle: the dispatch boundary
// resolved it once, and everything downstream reads the same scope.
func requestScope(ctx context.Context) binding.Scope {
	host, _ := ctx.Value(appHostKey{}).(appHost)
	return scopeOrRefusing(host.scope)
}

// scopeOrRefusing returns the scope to read from.
//
// A scope that was never filled — a server built by hand, without the
// composition root — refuses every family instead of calling into nothing,
// and it keeps the directory it was asked about.
func scopeOrRefusing(scope binding.Scope) binding.Scope {
	if scope.Filled() {
		return scope
	}
	return binding.RefusingScope(scope.Directory)
}
