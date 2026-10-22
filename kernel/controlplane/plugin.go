// SPDX-License-Identifier: MIT
package controlplane

import (
	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	"github.com/agezt/agezt/kernel/runtime"
)

// nativePluginReader maps the selected daemon manifest into the app port. It does
// not spawn, inspect or invoke plugin processes. The application's registration
// row is the contract type itself, so the manifest is handed over as it is —
// there is no field-by-field projection left to drift.
type nativePluginReader struct{ kernel *runtime.Kernel }

func (r nativePluginReader) Plugins() []appplugins.Registration {
	return r.kernel.Plugins()
}
