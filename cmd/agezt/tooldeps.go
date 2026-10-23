// SPDX-License-Identifier: MIT

package main

import (
	"io"

	"github.com/agezt/agezt/kernel/app/introspect"
	"github.com/agezt/agezt/kernel/app/overseer"
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
	}
}
