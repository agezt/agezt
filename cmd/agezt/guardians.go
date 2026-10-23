// SPDX-License-Identifier: MIT

package main

import (
	"github.com/agezt/agezt/kernel/app/guardians"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/builtinguardians"
)

// seedBuiltInGuardians seeds the daemon's internal guardian fleet — the
// health responder, the trust-ceiling sweeper and the rest.
//
// It is a named function (not an inline call in the boot step) because the
// wiring it performs is a CONTRACT with the seeder: the daemon hands over the
// contract's port, and a daemon that handed over nothing would silently seed
// no guardians at all — the boot banner would still report a fleet.
//
// A daemon with no kernel is refused by the seeder itself, so the boot step
// hears about it instead of panicking.
func seedBuiltInGuardians(k *runtime.Kernel) ([]builtinguardians.Seeded, error) {
	return builtinguardians.SeedAll(guardians.NewKernelHost(k), "")
}
