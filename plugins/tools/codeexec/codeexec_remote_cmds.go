// SPDX-License-Identifier: MIT

package codeexec

// Remote command-build helpers: remoteRuntimeCommand +
// remotePipInstallCommand + remoteRunCommand + quoteCommand.
// Carved out of codeexec_remote.go during the Day 185 god-file split
// so the main file can stay focused on the SSH/K8s/Modal invoke
// methods + their runners.
// Public API unchanged.

import (
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/kernel/executionprofile"
)

func remoteRuntimeCommand(lang, interp string) string {
	switch lang {
	case LangPython:
		base := strings.ToLower(filepath.Base(interp))
		if strings.HasPrefix(base, "python3") {
			return "python3"
		}
		return "python3"
	case LangNode:
		return "node"
	case LangDeno:
		return "deno"
	default:
		return filepath.Base(interp)
	}
}

func remotePipInstallCommand(runtime string, pkgs []string) string {
	args := []string{runtime, "-m", "pip", "install", "--target", pyDepsName, "--no-input", "--disable-pip-version-check", "--no-warn-script-location"}
	args = append(args, pkgs...)
	return quoteCommand(args)
}

func remoteRunCommand(lang, runtime, entry string, allowNet bool, hasDeps bool) string {
	switch lang {
	case LangPython:
		args := []string{runtime, entry}
		cmd := quoteCommand(args)
		if hasDeps {
			cmd = "PYTHONPATH=" + executionprofile.ShellQuote(pyDepsName) + " " + cmd
		}
		return cmd
	case LangDeno:
		args := []string{runtime, "run", "--quiet", "--no-prompt", "--allow-read=.", "--allow-write=.", "--allow-env"}
		if allowNet {
			args = append(args, "--allow-net")
		}
		args = append(args, entry)
		return quoteCommand(args)
	default:
		return quoteCommand([]string{runtime, entry})
	}
}

func quoteCommand(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, executionprofile.ShellQuote(a))
	}
	return strings.Join(out, " ")
}

