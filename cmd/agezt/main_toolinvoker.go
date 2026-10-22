// SPDX-License-Identifier: MIT

package main

import (
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/modules/reasoning"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// openAppKernel is shared by primary and tenant composition. Every Open binds
// the application entry to that kernel's own policy, audit and lookup ports.
//
// It also binds the reasoning module — the Council, the Conductor and the
// deep-research harness. The module cannot be imported by the kernel itself (a
// module may import another module's api package and nothing else — archcheck's
// principle P5), so the composition root builds it here, over the kernel's own
// ports, and injects it.
func openAppKernel(cfg kernelruntime.Config) (*kernelruntime.Kernel, error) {
	cfg.NewToolInvoker = apptools.NewInvoker
	k, err := kernelruntime.Open(cfg)
	if err != nil {
		return nil, err
	}
	k.SetReasoning(reasoning.New(k.ReasoningDependencies()))
	return k, nil
}
