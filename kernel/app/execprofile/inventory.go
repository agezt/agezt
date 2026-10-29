// SPDX-License-Identifier: MIT

package execprofile

import (
	"sort"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// ToolNames lists a kernel's registered tool names in a stable order.
func ToolNames(tools map[string]toolapi.Tool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
