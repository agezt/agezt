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
		// Identity, terminal, timezone and XDG dirs: CLIs misbehave without them
		// and none carries a secret.
		"USER": true, "LOGNAME": true, "SHELL": true, "TERM": true, "TZ": true, "LANGUAGE": true,
		"XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "XDG_CACHE_HOME": true, "XDG_RUNTIME_DIR": true,
		// Proxies: without them a child behind a corporate proxy cannot reach the
		// network at all. (A proxy URL may embed credentials; the child needs them
		// to use the proxy the operator configured.)
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true,
		// The ssh agent socket: git over ssh. The child can already read the
		// operator's HOME, so the socket adds nothing a hostile child lacked.
		"SSH_AUTH_SOCK": true,
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
