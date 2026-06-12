// SPDX-License-Identifier: MIT

//go:build linux

package creds

import (
	"os"
	"strings"
)

// machineID returns the systemd machine id (stable for the OS install), with
// the legacy dbus path as a fallback. "" when neither exists (e.g. a minimal
// container) — the caller then leaves the vault plaintext rather than deriving
// an unstable key.
func machineID() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(p); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return s
			}
		}
	}
	return ""
}
