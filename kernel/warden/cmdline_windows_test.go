// SPDX-License-Identifier: MIT

//go:build windows

package warden

import (
	"os/exec"
	"testing"
)

func TestFixupWindowsCmd_RawCmdLineForCmdC(t *testing.T) {
	c := exec.Command("cmd", "/C", `dir "C:\a b" /b`)
	fixupWindowsCmd(c)
	if c.SysProcAttr == nil || c.SysProcAttr.CmdLine == "" {
		t.Fatal("expected a raw CmdLine to be set for cmd /C")
	}
	want := `cmd /S /C "dir "C:\a b" /b"`
	if c.SysProcAttr.CmdLine != want {
		t.Errorf("CmdLine =\n  %q\nwant\n  %q", c.SysProcAttr.CmdLine, want)
	}
}

func TestFixupWindowsCmd_IgnoresNonCmd(t *testing.T) {
	// A direct interpreter invocation (code_exec path) must be left untouched.
	c := exec.Command("python", "-c", `print("hi")`)
	fixupWindowsCmd(c)
	if c.SysProcAttr != nil && c.SysProcAttr.CmdLine != "" {
		t.Errorf("non-cmd invocation must not get a raw CmdLine, got %q", c.SysProcAttr.CmdLine)
	}
	// Too few args is also a no-op.
	c2 := exec.Command("cmd")
	fixupWindowsCmd(c2)
	if c2.SysProcAttr != nil && c2.SysProcAttr.CmdLine != "" {
		t.Error("bare cmd must not get a raw CmdLine")
	}
}
