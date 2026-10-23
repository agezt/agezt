// SPDX-License-Identifier: MIT

package main

import (
	"io"

	"github.com/agezt/agezt/kernel/app/introspect"
	"github.com/agezt/agezt/kernel/app/overseer"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/modules/extensions/toolforge"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
)

// toolKernelDeps assembles the post-Open dependencies the registry's
// Configure phase injects into the built tools.
//
// It is a named function (not an inline literal in runDaemon) because the
// wiring it performs is a CONTRACT with the tools: the daemon hands over the
// app-layer implementations of the ports those tools read through. A port
// that is silently dropped here would leave the tool answering "not
// available" on every live daemon — invisible to every test that builds the
// tool by hand. The composition root's own test therefore asserts that each
// port this function supplies is present and answers the live kernel.
func toolKernelDeps(k *runtime.Kernel, baseDir string, stdout io.Writer) toolreg.KernelDeps {
	return toolreg.KernelDeps{
		K:               k,
		Bus:             k.Bus(),
		Artifacts:       k.ArtifactIndex(),
		Lake:            k.DataLake(),
		Journal:         k.Journal(),
		BaseDir:         baseDir,
		Stdout:          stdout,
		NetguardPublish: netguardPublish(k.Bus()),
		// The overseer tool steers the fleet through a contract port; the
		// implementation that answers it needs the live kernel and the board
		// store, so the daemon builds it here (the tool is a plugin and may
		// reach neither).
		Overseer: overseer.NewKernelSource(k, baseDir),
		// The introspect tool reads the daemon's own live state through a
		// contract port; the app-layer adapter gathers it from the live
		// kernel, because the tool is a plugin and may reach neither.
		Introspect: introspect.NewKernelSource(k),
		// The tool_forge tool reads the script-tool store through a contract
		// port; the daemon hands the live store over, because the tool is a
		// plugin and may not name the module that keeps it. A nil store must
		// stay a nil port, or the tool would call into a nil implementation
		// instead of reporting that the store is unavailable.
		Forge: forgeStore(k),
		// The workflow tool reads the workflow library through a contract
		// port; the daemon hands the live store over, for the same reason.
		Workflows: workflowStore(k),
	}
}

// workflowStore hands the live workflow library to the tool registry as the
// contract's port — the same nil-port rule as the forge store.
func workflowStore(k *runtime.Kernel) workflowapi.Store {
	if k == nil {
		return nil
	}
	s := k.Workflows()
	if s == nil {
		return nil
	}
	return s
}

// forgeStore hands the live script-tool store to the tool registry as the
// contract's port. It is a named function so the nil-port rule is stated once
// and can be tested on its own.
func forgeStore(k *runtime.Kernel) toolforgeapi.Store {
	if k == nil {
		return nil
	}
	return forgePort(k.ToolForge())
}

// forgePort turns the concrete store into the contract's port.
//
// A nil store must stay a NIL PORT: a port that wraps a nil implementation is
// not nil, so the tool would call into nothing instead of reporting that the
// store is unavailable. The check is on the concrete type on purpose — a
// nil *Store handed to an interface parameter is already a non-nil interface.
func forgePort(s *toolforge.Store) toolforgeapi.Store {
	if s == nil {
		return nil
	}
	return s
}
