// SPDX-License-Identifier: MIT
//
// kernel/controlplane settings helpers (configFieldNeedsKernelReload, setLiveEnv).
// Extracted from settings.go during Day 211 god-file refactor (#84).
// Public API unchanged.
package controlplane

import (
	"os"
)

func configFieldNeedsKernelReload(name string) bool {
	switch name {
	case "AGEZT_PROVIDER", "AGEZT_MODEL":
		return true
	default:
		return false
	}
}
func setLiveEnv(name, value string) {
	if value == "" {
		_ = os.Unsetenv(name)
		return
	}
	_ = os.Setenv(name, value)
}
