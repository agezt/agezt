// SPDX-License-Identifier: MIT
//
// Install path: InstallResult + installTimeout + Install + tail.
// Extracted from toolbox.go during the Day-203 god-file split.
// Public API unchanged.
package toolbox

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// InstallResult is one tool's install outcome (streamed per-tool by the caller).
type InstallResult struct {
	Tool       string `json:"tool"`
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped,omitempty"` // not installable on this host
	Manager    string `json:"manager,omitempty"`
	Command    string `json:"command,omitempty"`
	Version    string `json:"version,omitempty"` // version after a successful install
	OutputTail string `json:"output_tail,omitempty"`
	Error      string `json:"error,omitempty"`
}

// installTimeout bounds one package install — downloads + compile can be slow,
// so this is generous.
const installTimeout = 20 * time.Minute

// Install runs the resolved package-manager command for one named tool at the
// host level and returns the outcome (re-probing the version on success).
// Unknown names / un-installable tools return Skipped, never an error, so a
// batch keeps going. progress, if non-nil, is unused here (the caller streams).
func Install(ctx context.Context, name string) InstallResult {
	goos := runtime.GOOS
	t, ok := byName(name)
	if !ok {
		return InstallResult{Tool: name, Skipped: true, Error: "unknown tool"}
	}
	managers := DetectManagers(goos)
	r, ok := ResolveInstall(t, goos, managers)
	res := InstallResult{Tool: name, Manager: r.Manager, Command: strings.Join(r.Install, " ")}
	if !ok || len(r.Install) == 0 {
		res.Skipped = true
		res.Error = "no install recipe for this host"
		return res
	}
	cctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, r.Install[0], r.Install[1:]...)
	out, err := cmd.CombinedOutput()
	res.OutputTail = tail(string(out), 1200)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	res.Version = probeVersion(ctx, t.bin(goos), t.versionArgs())
	return res
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
