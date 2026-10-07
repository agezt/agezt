// SPDX-License-Identifier: MIT
package controlplane

import (
	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	"github.com/agezt/agezt/kernel/runtime"
)

// nativePluginReader maps the selected daemon manifest into the app port. It does
// not spawn, inspect or invoke plugin processes.
type nativePluginReader struct{ kernel *runtime.Kernel }

func (r nativePluginReader) Plugins() []appplugins.Registration {
	plugins := r.kernel.Plugins()
	rows := make([]appplugins.Registration, len(plugins))
	for i, p := range plugins {
		rows[i] = appplugins.Registration{Prefix: p.Prefix, Path: p.Path, Args: p.Args, ToolCount: p.ToolCount, HashPinned: p.HashPinned, AllowedTools: p.AllowedTools}
	}
	return rows
}
