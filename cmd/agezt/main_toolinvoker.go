// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"time"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/modules/agents/reaper"
	"github.com/agezt/agezt/kernel/modules/artifacts/datalake"
	"github.com/agezt/agezt/kernel/modules/reasoning"
	"github.com/agezt/agezt/kernel/modules/work/seat"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// openAppKernel is shared by primary and tenant composition. Every Open binds
// the application entry to that kernel's own policy, audit and lookup ports.
//
// It also binds two modules — the reasoning module (the Council, the Conductor
// and the deep-research harness) and the agents module's reaper (the read-only
// fleet health scan). A module cannot be imported by the kernel itself (a
// module may import another module's api package and nothing else — archcheck's
// principle P5), so the composition root builds them here, over the kernel's own
// ports, and injects them.
//
// The seat catalog is bound the same way: the daemon opens the store (the file
// is the daemon's business) and hands the kernel the contract's port. A store
// that cannot be opened is a startup error, exactly as it was when the kernel
// opened it itself.
func openAppKernel(cfg kernelruntime.Config) (*kernelruntime.Kernel, error) {
	cfg.NewToolInvoker = apptools.NewInvoker
	if cfg.Seats == nil {
		seats, err := seat.OpenStore(filepath.Join(cfg.BaseDir, "seats"))
		if err != nil {
			return nil, err
		}
		cfg.Seats = seats
	}
	if cfg.Lake == nil {
		lake, err := datalake.Open(cfg.BaseDir, func() int64 { return time.Now().UnixMilli() })
		if err != nil {
			return nil, err
		}
		// Seed the built-in Personal Data Lake collections (M835) — expenses,
		// calendar, tasks, notes, habits, bookmarks, contacts. Idempotent
		// (EnsureCollection skips existing ones) and best-effort: a seed hiccup
		// must not block boot, and the next start retries.
		//
		// The seeding happens HERE, while the daemon still holds the concrete
		// lake: seeding is not part of the port the host is given.
		_, _ = lake.SeedBuiltins("system")
		cfg.Lake = lake
	}
	k, err := kernelruntime.Open(cfg)
	if err != nil {
		return nil, err
	}
	k.SetReasoning(reasoning.New(k.ReasoningDependencies()))
	k.SetReaperScanner(reaper.New(k.ReaperPorts()))
	return k, nil
}
