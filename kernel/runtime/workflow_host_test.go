// SPDX-License-Identifier: MIT

package runtime_test

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"github.com/agezt/agezt/kernel/runtime"
)

// openWorkflowKernel opens the kernel the way the COMPOSITION ROOT does
// (W3.48): the workflow store is the daemon's file, and the engine is the
// daemon's too — built over that store and over the host's own ports (the
// kernel's governed paths), and handed over as the contract's engine port.
//
// A kernel opened without an engine refuses every workflow operation with
// "the workflow engine is not available on this daemon" — the refusal is the
// answer, not a panic (see TestTheWorkflowRefusesWithoutAnEngine).
func openKernelWithWorkflows(t *testing.T, cfg runtime.Config) (*runtime.Kernel, error) {
	t.Helper()
	store, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
	if err != nil {
		return nil, err
	}
	cfg.Workflows = store
	k, err := runtime.Open(cfg)
	if err != nil {
		return nil, err
	}
	k.SetWorkflowEngine(workflow.NewEngine(store, k.WorkflowPorts()))
	return k, nil
}
