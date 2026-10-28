// SPDX-License-Identifier: MIT
package controlplane

import (
	appbasedir "github.com/agezt/agezt/kernel/app/basedir"
	appconfig "github.com/agezt/agezt/kernel/app/config"
	appedict "github.com/agezt/agezt/kernel/app/edict"
	appprovider "github.com/agezt/agezt/kernel/app/provider"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/runtime"
	"os"
)

// nativeConfigReader selects live daemon fields; presentation belongs to app/config.
//
// The tool count is the TOOLS family's answer: the reader asks the tools
// ports for it rather than reaching for the registry itself. The ask-policy
// answer comes from the POLICY family's ports and the routing view from the
// PROVIDER family's ports, for the same reason.
type nativeConfigReader struct {
	kernel   *runtime.Kernel
	tools    apptools.Ports
	edict    appedict.Ports
	provider appprovider.Ports
	basedir  appbasedir.Ports
}

func (r nativeConfigReader) BaseDir() string       { return r.basedir.Dir() }
func (r nativeConfigReader) Model() string         { return r.kernel.Model() }
func (r nativeConfigReader) SystemPromptSet() bool { return r.kernel.System() != "" }
func (r nativeConfigReader) ToolCount() int        { return r.tools.Count() }
func (r nativeConfigReader) PluginCount() int      { return len(r.kernel.Plugins()) }
func (r nativeConfigReader) AskPolicy() string     { return r.edict.AskPolicy() }
func (r nativeConfigReader) Routing() (appconfig.RoutingReader, bool) {
	view, ok := r.provider.Routing()
	return view, ok
}
func nativeConfigEnvPresent(name string) bool { _, present := os.LookupEnv(name); return present }
