// SPDX-License-Identifier: MIT

package execprofile

import (
	"sort"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/runtime"
)

// Inventory builds a kernel's execution-profile inventory from its tools and
// warden and the host's remote backend configuration, read now.
func Inventory(k *runtime.Kernel) executionprofile.Inventory {
	return executionprofile.Build(executionprofile.Options{
		Tools:   ToolNames(k.Tools()),
		Warden:  k.Warden(),
		SSH:     executionprofile.SSHConfigFromEnv(),
		K8s:     executionprofile.K8sConfigFromEnv(),
		Modal:   executionprofile.ModalConfigFromEnv(),
		Daytona: executionprofile.DaytonaConfigFromEnv(),
	})
}

// ToolNames lists a kernel's registered tool names in a stable order.
func ToolNames(tools map[string]toolapi.Tool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
