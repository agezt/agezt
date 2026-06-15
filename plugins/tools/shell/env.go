// SPDX-License-Identifier: MIT

package shell

import (
	"os"
	"strings"
)

// scrubEnv builds the child environment for a shell command (M957): an allowlist
// of harmless OS variables a shell needs — PATH (so external programs resolve),
// the Windows system vars cmd.exe relies on (SystemRoot/COMSPEC/… — without them
// even built-ins fail with "The filename, directory name, or volume label syntax
// is incorrect"), and locale — with HOME/TMP pointed at the work dir. Every
// secret-shaped variable and the entire AGEZT_* namespace (API keys, provider
// creds, tokens) is dropped, so a model-issued command can never read the
// daemon's secrets.
//
// This mirrors codeexec.scrubEnv (the load-bearing safety property of code_exec):
// warden defaults a nil Spec.Env to an EMPTY environment to avoid leaking the
// daemon's secrets, but an empty env breaks cmd.exe entirely (no PATH/SystemRoot)
// — the cause of the shell tool's ~66% Windows error rate. A scrubbed host env
// is the documented "caller wants inheritance, explicitly" path, secret-safe per
// the non-negotiable secret-scrub posture.
func scrubEnv(dir string) []string {
	if dir == "" {
		dir = os.TempDir()
	}
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"NUMBER_OF_PROCESSORS": true, "PROCESSOR_ARCHITECTURE": true,
		// User/identity vars many tools need to locate config (git, ssh, npm…)
		// — none secret-bearing; secret-shaped names are still dropped below.
		"USERPROFILE": true, "USERNAME": true, "HOMEDRIVE": true, "HOMEPATH": true,
		"LANG": true,
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
	// Point HOME / temp at the work dir so scratch files land in the workspace
	// rather than the daemon user's real home.
	out = append(out,
		"HOME="+dir,
		"TMPDIR="+dir,
		"TEMP="+dir,
		"TMP="+dir,
	)
	return out
}

// isSecretName reports whether an env var name looks secret-bearing and must
// never be forwarded into a model-issued command. Mirrors codeexec.isSecretName.
func isSecretName(up string) bool {
	for _, frag := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CRED", "AWS_", "AGEZT_"} {
		if strings.Contains(up, frag) {
			return true
		}
	}
	return false
}
