// SPDX-License-Identifier: MIT

// Package envscrub builds child-process environments that keep ordinary OS
// launch variables while dropping daemon/provider secrets.
package envscrub

import (
	"os"
	"strings"
)

// Scrubbed returns a child environment suitable for operator-configured helper
// processes. It keeps OS variables needed to launch shells and CLIs, but drops
// AGEZT_* and secret-shaped names inherited from the daemon.
func Scrubbed() []string {
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"NUMBER_OF_PROCESSORS": true, "PROCESSOR_ARCHITECTURE": true,
		"LANG": true,
		// User/config dirs are needed by external CLIs such as Codex, Claude, git,
		// ssh, npm, and package managers. Secret-shaped names are still dropped.
		"HOME": true, "USERPROFILE": true, "USERNAME": true, "HOMEDRIVE": true, "HOMEPATH": true,
		"APPDATA": true, "LOCALAPPDATA": true, "PROGRAMDATA": true,
		"TEMP": true, "TMP": true, "TMPDIR": true,
		"PROGRAMFILES": true, "PROGRAMFILES(X86)": true, "PROGRAMW6432": true,
	}
	var out []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		up := strings.ToUpper(name)
		if IsSecretName(up) {
			continue
		}
		if allow[up] || strings.HasPrefix(up, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

// With returns base plus explicit key=value entries. Use this only for values
// intentionally handed to the child, such as a task payload.
func With(base []string, kvs ...string) []string {
	out := append([]string(nil), base...)
	out = append(out, kvs...)
	return out
}

// IsSecretName reports whether an environment variable name must not be
// inherited from the daemon into child processes.
func IsSecretName(up string) bool {
	up = strings.ToUpper(up)
	for _, frag := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CRED", "AWS_", "AGEZT_"} {
		if strings.Contains(up, frag) {
			return true
		}
	}
	return false
}
