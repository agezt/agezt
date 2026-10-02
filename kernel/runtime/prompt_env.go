// SPDX-License-Identifier: MIT

package runtime

// Provenance: Prompt environment: injectHostEnvironment. Code extracted from
//             prompt.go during the Day-73 god-file split. Public API unchanged.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// Config.EnvironmentInject is set.
func (k *Kernel) injectHostEnvironment(system string, tools map[string]toolapi.Tool) string {
	if k.cfg.EnvironmentInject {
		system = injectEnvironment(system, k.cfg.WorkspaceRoot, tools, time.Now())
	}
	return system
}
