// SPDX-License-Identifier: MIT

package controlplane

// Past-runs enumeration: the host's binding of the journal to the
// application's run fold. The FOLD — what a run row is, which markers end a
// run, what its phase, spend, model and answer excerpt are — lives in the
// application layer (`kernel/app/runs/collect.go`); what stays here is the
// host's data access: the journal.

import (
	"time"

	appjournal "github.com/agezt/agezt/kernel/app/journal"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	"github.com/agezt/agezt/kernel/runtime"
)

// runReads binds the app run reads to the given kernel's folded runs.
func (s *Server) runReads(k *runtime.Kernel) *appruns.Service {
	return appruns.New(func() (map[string]appruns.Run, error) {
		return s.runFold(k)
	}, time.Now)
}

// runFold walks the given kernel's journal and folds it into the app layer's
// run rows. The run operation enriches its terminal answer with the run's own
// iters, spend and model from this fold.
func (s *Server) runFold(k *runtime.Kernel) (map[string]appruns.Run, error) {
	return appruns.Collect(appjournal.PortsFromKernel(k).Events.Range)
}
