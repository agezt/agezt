// SPDX-License-Identifier: MIT
package controlplane

import (
	appconfig "github.com/agezt/agezt/kernel/app/config"
	"github.com/agezt/agezt/kernel/runtime"
	"os"
)

// nativeConfigReader selects live daemon fields; presentation belongs to app/config.
type nativeConfigReader struct{ kernel *runtime.Kernel }

func (r nativeConfigReader) BaseDir() string       { return r.kernel.BaseDir() }
func (r nativeConfigReader) Model() string         { return r.kernel.Model() }
func (r nativeConfigReader) SystemPromptSet() bool { return r.kernel.System() != "" }
func (r nativeConfigReader) ToolCount() int        { return len(r.kernel.Tools()) }
func (r nativeConfigReader) PluginCount() int      { return len(r.kernel.Plugins()) }
func (r nativeConfigReader) AskPolicy() string     { return r.kernel.Edict().AskPolicy().String() }
func (r nativeConfigReader) Routing() (appconfig.RoutingReader, bool) {
	view, ok := r.kernel.Provider().(appconfig.RoutingReader)
	return view, ok
}
func nativeConfigEnvPresent(name string) bool { _, present := os.LookupEnv(name); return present }
