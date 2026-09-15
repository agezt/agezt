// SPDX-License-Identifier: MIT
//
// cmd/agt doctor environment checks: checkBaseDir + checkVersionSkew +
// checkTools + checkHalt.
// Extracted from doctor_ops.go during Day 211 god-file refactor (#54).
// Public API unchanged.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agezt/agezt/internal/brand"
)

func checkBaseDir(base string, baseErr error) doctorCheck {
	const name = "base directory"
	if baseErr != nil {
		return fail(name, baseErr.Error(), "set AGEZT_HOME or check filesystem permissions")
	}
	info, err := os.Stat(base)
	if os.IsNotExist(err) {
		return warn(name, base+" (not created yet)",
			fmt.Sprintf("run `%s` once to initialise it", brand.Binary))
	}
	if err != nil {
		return fail(name, err.Error(), "check filesystem permissions")
	}
	if !info.IsDir() {
		return fail(name, base+" exists but is not a directory", "remove the file or set AGEZT_HOME")
	}
	// Prove writability rather than guessing from mode bits.
	probe := filepath.Join(base, ".doctor-probe")
	if werr := os.WriteFile(probe, []byte("ok"), 0o600); werr != nil {
		return fail(name, base+" (not writable: "+werr.Error()+")", "fix ownership/permissions on the base dir")
	}
	_ = os.Remove(probe)
	return ok(name, base+" (writable)")
}
func checkVersionSkew(status map[string]any) doctorCheck {
	const name = "version skew"
	daemonVer, _ := status["daemon"].(string)
	daemonProto := intOfStatus(status["protocol"])
	if daemonVer == brand.Version && daemonProto == int64(brand.ProtocolVersion) {
		return ok(name, fmt.Sprintf("client and daemon aligned (%s, protocol v%d)", brand.Version, brand.ProtocolVersion))
	}
	return warn(name,
		fmt.Sprintf("client %s/v%d vs daemon %s/v%d", brand.Version, brand.ProtocolVersion, daemonVer, daemonProto),
		fmt.Sprintf("restart the daemon to align (`%s shutdown` then `%s`)", brand.CLI, brand.Binary))
}
func checkTools(status map[string]any) doctorCheck {
	const name = "tools"
	n := intOfStatus(status["tools"])
	if n == 0 {
		return warn(name, "0 registered", "no capabilities available — check tool plugins / AGEZT_TOOLS")
	}
	return ok(name, fmt.Sprintf("%d registered", n))
}
func checkHalt(status map[string]any) doctorCheck {
	const name = "halt state"
	if halted, _ := status["halted"].(bool); halted {
		return warn(name, "system is HALTED", fmt.Sprintf("resume work with `%s resume`", brand.CLI))
	}
	return ok(name, "running")
}
