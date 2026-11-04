// SPDX-License-Identifier: MIT

package testfixtures

// Provenance: the shared test host helper for the STANDING LIBRARY (W3.76x).
//             The library's file is the daemon's and the kernel holds the
//             contract's store port — and the library's LIFECYCLE is the
//             standing package's, so the host injects the BUILDER the way the
//             composition root does.

import (
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/standing"
)

// StandingBuilder builds the standing library's service over the host's store
// — the composition root's wiring (W3.76x), for the tests that need the
// library's lifecycle.
func StandingBuilder(store standingapi.Store, publish func(event.Spec)) standingapi.Lifecycle {
	return standing.NewService(standing.Dependencies{Store: store, Publish: publish})
}
