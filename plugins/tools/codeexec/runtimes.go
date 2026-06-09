// SPDX-License-Identifier: MIT

package codeexec

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Supported language identifiers.
const (
	LangPython = "python"
	LangNode   = "node"
	LangDeno   = "deno"
)

// DetectRuntimes resolves the interpreters available on this host to absolute
// paths (so PATH isn't needed to FIND them at run time). Only languages whose
// interpreter resolves are returned — the tool exposes exactly what can run.
func DetectRuntimes() map[string]string {
	out := map[string]string{}
	if p := lookAny(pythonCandidates(runtime.GOOS)...); p != "" {
		out[LangPython] = p
	}
	if p := lookAny("node"); p != "" {
		out[LangNode] = p
	}
	if p := lookAny("deno"); p != "" {
		out[LangDeno] = p
	}
	return out
}

// pythonCandidates is the interpreter-name preference order for the host OS. On
// Windows we try `python` (a real install, e.g. C:\PythonXX\python.exe) BEFORE
// `python3`, because `python3` there is usually the Microsoft Store shim
// (…\WindowsApps\python3.exe) which can trigger the Store auto-installer mid-run
// and pollute the program's output with "Installing Python…" chatter. Elsewhere
// `python3` is the canonical name and `python` may be absent or python2, so we
// keep the usual order.
func pythonCandidates(goos string) []string {
	if goos == "windows" {
		return []string{"python", "python3"}
	}
	return []string{"python3", "python"}
}

func lookAny(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// sortedLangs returns the available language ids in a stable order, for the
// schema enum and the daemon banner.
func sortedLangs(rt map[string]string) []string {
	out := make([]string, 0, len(rt))
	for l := range rt {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// entryName is the entrypoint filename the code is written to per language.
func entryName(lang string) string {
	switch lang {
	case LangPython:
		return "main.py"
	case LangNode:
		return "main.js"
	case LangDeno:
		return "main.ts"
	}
	return "main.txt"
}

// buildArgv assembles the argv to run the entrypoint in dir. For Deno we hand
// it an explicit permission set: filesystem read/write CONFINED to the work dir
// and env access (env is already scrubbed), plus network only when granted —
// this is a real OS-level jail on every platform, including Windows. We
// deliberately do NOT pass --allow-run, so a Deno script can't shell out to
// escape the fs confinement. Python/Node take no such flags (their isolation is
// whatever the warden profile provides — real on Linux+namespace, workdir/env/
// limits only elsewhere).
func buildArgv(interp, lang, entry string, dir string, allowNet bool) []string {
	if lang == LangDeno {
		args := []string{
			interp, "run", "--no-prompt",
			"--allow-read=" + dir,
			"--allow-write=" + dir,
			"--allow-env",
		}
		if allowNet {
			args = append(args, "--allow-net")
		}
		return append(args, entry)
	}
	return []string{interp, entry}
}

// scrubEnv builds the child environment: an allowlist of harmless OS variables
// the interpreter needs (PATH, Windows system vars, locale) with HOME/TMP
// pointed at the work dir — and NOTHING else. Every secret-shaped variable and
// the entire AGEZT_* namespace (API keys, provider creds, tokens) is dropped so
// model-written code can never read the daemon's secrets. This is the load-
// bearing safety property of the whole tool.
func scrubEnv(dir string) []string {
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"NUMBER_OF_PROCESSORS": true, "PROCESSOR_ARCHITECTURE": true,
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
	// Point HOME / temp at the work dir so scripts that write scratch files keep
	// them inside the sandbox rather than the daemon user's real home.
	out = append(out,
		"HOME="+dir,
		"TMPDIR="+dir,
		"TEMP="+dir,
		"TMP="+dir,
		"PYTHONDONTWRITEBYTECODE=1",  // no __pycache__ litter in the work dir
		"PYLAUNCHER_ALLOW_INSTALL=0", // never let the Windows py launcher auto-install Python mid-run
	)
	return out
}

// isSecretName reports whether an env var name looks secret-bearing and must
// never be forwarded into untrusted code.
func isSecretName(up string) bool {
	for _, frag := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CRED", "AWS_", "AGEZT_"} {
		if strings.Contains(up, frag) {
			return true
		}
	}
	return false
}

// slug sanitizes a project name into a single safe path segment (lowercase
// kebab, alnum + dash). It can never contain a separator or "..", so a project
// dir can't escape sandboxRoot/projects. Empty input falls back to "project".
func slug(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "project"
	}
	return out
}

// sanitizeRelFile validates an extra-file name supplied by the model: it must be
// a relative path that stays within the work dir. Rejects absolute paths and any
// ".." traversal. Returns the cleaned, slash-form relative path.
func sanitizeRelFile(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.IsAbs(name) || strings.Contains(name, "\x00") {
		return "", false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if clean == ".." || clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", false
	}
	// A Windows drive-relative path (C:foo) survives Clean; reject any colon.
	if strings.Contains(clean, ":") {
		return "", false
	}
	return clean, true
}
