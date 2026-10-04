// SPDX-License-Identifier: MIT

package controlplane

import "testing"

func TestFileCommandsArePrimaryMutations(t *testing.T) {
	for _, command := range []string{CmdFileMkdir, CmdFileRename, CmdFileDelete} {
		spec, ok := commandRegistry[command]
		if !ok || spec.ReadOnly || spec.TenantAllowed || spec.TenantRouted || spec.Streaming != StreamNone {
			t.Errorf("workspace command must be an audited primary-only mutation: %q %+v", command, spec)
		}
	}
}
