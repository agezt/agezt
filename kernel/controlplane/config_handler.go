// SPDX-License-Identifier: MIT
package controlplane

import (
	appbasedir "github.com/agezt/agezt/kernel/app/basedir"
	appconfig "github.com/agezt/agezt/kernel/app/config"
	appedict "github.com/agezt/agezt/kernel/app/edict"
	apppersona "github.com/agezt/agezt/kernel/app/persona"
	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	appprovider "github.com/agezt/agezt/kernel/app/provider"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"os"
)

// nativeConfigReader selects live daemon fields; presentation belongs to app/config.
//
// The tool count is the TOOLS family's answer: the reader asks the tools
// ports for it rather than reaching for the registry itself. The ask-policy
// answer comes from the POLICY family's ports and the routing view from the
// PROVIDER family's ports, for the same reason. The plugin count is the
// PLUGINS family's answer, for the same reason again.
//
// And the daemon's default IDENTITY — the model and whether a prompt is set —
// is the PERSONA family's answer, for the same reason again. The reader holds
// no kernel handle at all: every field it reports belongs to a family, and it
// asks each of them.
type nativeConfigReader struct {
	persona  apppersona.Ports
	tools    apptools.Ports
	plugins  appplugins.Ports
	edict    appedict.Ports
	provider appprovider.Ports
	basedir  appbasedir.Ports
}

func (r nativeConfigReader) BaseDir() string       { return r.basedir.Dir() }
func (r nativeConfigReader) Model() string         { return r.persona.Identity.DefaultModel() }
func (r nativeConfigReader) SystemPromptSet() bool { return r.persona.Identity.PromptSet() }
func (r nativeConfigReader) ToolCount() int        { return r.tools.Count() }
func (r nativeConfigReader) PluginCount() int      { return len(r.plugins.Manifest()) }
func (r nativeConfigReader) AskPolicy() string     { return r.edict.AskPolicy() }
func (r nativeConfigReader) Routing() (appconfig.RoutingReader, bool) {
	view, ok := r.provider.Routing()
	return view, ok
}
func nativeConfigEnvPresent(name string) bool { _, present := os.LookupEnv(name); return present }
