// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"github.com/agezt/agezt/kernel/runtime"
)

// withWorkflows opens the workflow library the composition root would open
// (W3.48) and hands the kernel the contract's host port — and the ENGINE,
// because the engine is the daemon's too: it is built over that store and over
// the host's own ports (the kernel's governed paths).
//
// The library's file is the DAEMON's: the kernel holds the ports. This test
// is such a host.
//
// The helper returns the KERNEL and not the config, because the engine cannot
// be wired without the kernel's own ports.
func withWorkflows(t *testing.T, cfg runtime.Config) (*runtime.Kernel, error) {
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
