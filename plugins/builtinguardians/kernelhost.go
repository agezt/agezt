// SPDX-License-Identifier: MIT

package builtinguardians

import (
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
)

// kernelHost adapts the live *runtime.Kernel to the seeder's Host. Each write
// goes through the kernel's own stores, so seeded guardians, standing orders,
// and schedules are persisted and journaled exactly like operator-created ones.
// Schedules are tagged source "system" so SyncEnv (which only replaces "env"
// entries) never prunes them and the UI can tell them apart.
type kernelHost struct{ k *kernelruntime.Kernel }

// NewKernelHost builds the kernel-backed Host.
func NewKernelHost(k *kernelruntime.Kernel) Host { return &kernelHost{k: k} }

func (h *kernelHost) Agents() []roster.Profile { return h.k.Roster().List() }

func (h *kernelHost) AddAgent(p roster.Profile) (roster.Profile, error) {
	return h.k.AddProfile(p)
}

func (h *kernelHost) AddStanding(o standing.Order) (standing.Order, error) {
	return h.k.Standing().Add(o)
}

func (h *kernelHost) AddInterval(intent string, interval time.Duration, model string) (cadence.Entry, error) {
	return h.k.Schedules().Add(intent, interval, model, "system", time.Now())
}

func (h *kernelHost) AddDaily(intent string, atMinutes int, model string) (cadence.Entry, error) {
	// days=0 → every day; tz="" → daemon local.
	return h.k.Schedules().AddDaily(intent, atMinutes, 0, "", model, "system", time.Now())
}
