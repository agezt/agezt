// SPDX-License-Identifier: MIT

// Package mcp: env helpers for the MCP client (appendEnv merges a key/value
// map into a string slice; scrubbedEnv returns the daemon environment with
// secret-shaped vars dropped; isSecretName matches env-var names that look
// like credentials). Extracted from client.go during the Day-211 god-file
// split. Public API unchanged.
package mcp


import (
	"os"
	"strings"
)
func appendEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

// scrubbedEnv builds the child environment: harmless OS variables only
// (PATH, Windows system vars, per-user dirs npm-style launchers need,
// locale) — never AGEZT_* or secret-shaped variables. Mirrors the code-exec
// sandbox's scrub; this is the load-bearing safety property of attach.
func scrubbedEnv() []string {
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"NUMBER_OF_PROCESSORS": true, "PROCESSOR_ARCHITECTURE": true,
		"LANG": true, "HOME": true, "USERPROFILE": true,
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
		if isSecretName(up) {
			continue
		}
		if allow[up] || strings.HasPrefix(up, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

// isSecretName mirrors the code-exec sandbox's rule: anything secret-shaped
// — and the whole AGEZT_* namespace — never reaches a spawned server.
func isSecretName(up string) bool {
	for _, frag := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CRED", "AWS_", "AGEZT_"} {
		if strings.Contains(up, frag) {
			return true
		}
	}
	return false
}
