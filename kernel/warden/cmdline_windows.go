// SPDX-License-Identifier: MIT

//go:build windows

package warden

import (
	"os/exec"
	"strings"
	"syscall"
)

// fixupWindowsCmd makes a `cmd /C <command>` invocation pass the command to
// cmd.exe VERBATIM (M958). Go's os/exec escapes each arg for the MSVC C-runtime
// rules, but cmd.exe does NOT follow those rules: it has its own /C quote
// handling, so a command containing double-quotes (e.g. `dir "C:\a b" /b`) comes
// out mangled — "The filename, directory name, or volume label syntax is
// incorrect" or the whole command read as one quoted program name. This was the
// shell tool's dominant residual Windows error after the env fix (M957).
//
// The robust form is `cmd /S /C "<command>"`: with /S, cmd strips exactly the
// first and last quote of the string after /C and runs the remainder as-is —
// so embedded quotes survive intact. We set SysProcAttr.CmdLine directly, which
// tells os/exec to use this raw command line instead of re-escaping Args. The
// child binary is still cmd.Path (resolved from Args[0]).
func fixupWindowsCmd(cmd *exec.Cmd) {
	if cmd == nil || len(cmd.Args) < 3 {
		return
	}
	base := strings.ToLower(cmd.Args[0])
	isCmd := base == "cmd" || base == "cmd.exe" ||
		strings.HasSuffix(base, `\cmd.exe`) || strings.HasSuffix(base, `/cmd.exe`)
	if !isCmd {
		return
	}
	if strings.ToLower(cmd.Args[1]) != "/c" {
		return
	}
	command := strings.Join(cmd.Args[2:], " ")
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CmdLine = `cmd /S /C "` + command + `"`
}
