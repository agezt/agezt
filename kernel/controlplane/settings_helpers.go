package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane settings helpers
//             (setLiveEnv; reload classification moved to app/settings). Extracted from settings.go
//             during Day 211 god-file refactor (#84). Public API unchanged.

import (
	"os"
)

func setLiveEnv(name, value string) {
	if value == "" {
		_ = os.Unsetenv(name)
		return
	}
	_ = os.Setenv(name, value)
}
