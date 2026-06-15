// SPDX-License-Identifier: MIT

//go:build !windows

package warden

import "os/exec"

// fixupWindowsCmd is a no-op off Windows: sh -c on Unix passes the command as a
// single arg that the standard escaping handles correctly. (M958)
func fixupWindowsCmd(_ *exec.Cmd) {}
