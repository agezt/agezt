// SPDX-License-Identifier: MIT

// Prompt environment: injectHostEnvironment.
// Code extracted from prompt.go during the Day-73 god-file split. Public API unchanged.
package runtime


import (
	"github.com/agezt/agezt/kernel/agent"
	"time"
)


// Config.EnvironmentInject is set.
func (k *Kernel) injectHostEnvironment(system string, tools map[string]agent.Tool) string {
	if k.cfg.EnvironmentInject {
		system = injectEnvironment(system, k.cfg.WorkspaceRoot, tools, time.Now())
	}
	return system
}
