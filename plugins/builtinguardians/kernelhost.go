// SPDX-License-Identifier: MIT

package builtinguardians

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
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

func (h *kernelHost) UpdateAgent(ref string, mutate func(*roster.Profile)) (roster.Profile, bool, error) {
	return h.k.UpdateProfile(ref, mutate)
}

func (h *kernelHost) StandingOrders() []standingapi.Order {
	return h.k.Standing().List()
}

func (h *kernelHost) UpdateStanding(id string, mutate func(*standingapi.Order)) (standingapi.Order, bool, error) {
	return h.k.UpdateStanding(id, mutate)
}

func (h *kernelHost) AddStanding(o standingapi.Order) (standingapi.Order, error) {
	return h.k.Standing().Add(o)
}

func (h *kernelHost) SetStandingEnabled(id string, enabled bool) (standingapi.Order, error) {
	return h.k.SetStandingEnabled(id, enabled)
}

func (h *kernelHost) Schedules() []cadenceapi.Entry {
	return h.k.Schedules().List()
}

func (h *kernelHost) Reschedule(id string, mode string, interval time.Duration, atMinutes, days int) (bool, error) {
	return h.k.Schedules().Reschedule(id, mode, interval, atMinutes, 0, days, "", time.Time{}, time.Now())
}

func (h *kernelHost) AddInterval(intent string, interval time.Duration, model, agent string) (cadenceapi.Entry, error) {
	e, err := h.k.Schedules().Add(intent, interval, model, "system", time.Now())
	if err != nil || agent == "" {
		return e, err
	}
	if _, err := h.k.Schedules().SetAgent(e.ID, agent); err != nil {
		return cadenceapi.Entry{}, err
	}
	e.Agent = agent
	return e, nil
}

func (h *kernelHost) AddDaily(intent string, atMinutes int, model, agent string) (cadenceapi.Entry, error) {
	// days=0 → every day; tz="" → daemon local.
	e, err := h.k.Schedules().AddDaily(intent, atMinutes, 0, "", model, "system", time.Now())
	if err != nil || agent == "" {
		return e, err
	}
	if _, err := h.k.Schedules().SetAgent(e.ID, agent); err != nil {
		return cadenceapi.Entry{}, err
	}
	e.Agent = agent
	return e, nil
}
