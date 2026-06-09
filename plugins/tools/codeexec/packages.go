// SPDX-License-Identifier: MIT

package codeexec

import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/warden"
)

// pyDepsName is the per-workspace directory pip installs into (a `pip install
// --target` site). Under a project dir it persists across calls; under an
// ephemeral run dir it's discarded with the run.
const pyDepsName = ".deps"

// pipInstallTimeout bounds dependency installation — generous, since fetching and
// building wheels over the network can be slow.
const pipInstallTimeout = MaxTimeout

// validatePackages cleans and checks a pip requirement list. The warden runs pip
// via exec (no shell), so the only real hazard is a "package" that's actually a
// pip FLAG (e.g. "--index-url=evil"); we reject anything starting with "-" or
// carrying whitespace/NUL. Normal requirement specs — name, version pins
// (requests==2.31.0, pandas>=2), and extras (requests[security]) — are allowed.
func validatePackages(pkgs []string) ([]string, error) {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "-") {
			return nil, fmt.Errorf("illegal package %q (looks like a pip flag, not a package)", p)
		}
		if strings.ContainsAny(p, " \t\r\n\x00") {
			return nil, fmt.Errorf("illegal package name %q (whitespace not allowed)", p)
		}
		out = append(out, p)
	}
	return out, nil
}

// pipInstall installs pkgs into depsDir via `python -m pip install --target`.
// Network-using, time-bounded, scrubbed env, confined to the work dir — the same
// isolation envelope as the program run.
func pipInstall(ctx context.Context, w warden.Engine, interp, dir, depsDir string, pkgs []string, profile warden.Profile) (*warden.Result, error) {
	argv := append([]string{
		interp, "-m", "pip", "install",
		"--target", depsDir,
		"--no-input",
		"--disable-pip-version-check",
		"--no-warn-script-location",
	}, pkgs...)
	return w.Run(ctx, warden.Spec{
		Profile: profile,
		Argv:    argv,
		WorkDir: dir,
		Env:     scrubEnv(dir),
		Limits: warden.Limits{
			Timeout:           pipInstallTimeout,
			MaxOutputBytes:    MaxOutputBytes,
			CPUSeconds:        limitCPUSeconds,
			AddressSpaceBytes: limitAddressSpaceByte,
			MaxOpenFiles:      4096, // pip opens many files unpacking wheels
			MaxFileSizeBytes:  limitMaxFileSizeBytes,
		},
		Actor:         "tool.code_exec",
		CorrelationID: warden.CorrelationFrom(ctx),
	})
}

// installTail returns the last chunk of a failed install's output for the error
// message — the tail is where pip prints what actually went wrong.
func installTail(res *warden.Result) string {
	out := string(res.Stdout)
	if len(res.Stderr) > 0 {
		out += "\n" + string(res.Stderr)
	}
	out = strings.TrimSpace(out)
	const max = 2000
	if len(out) > max {
		out = "…" + out[len(out)-max:]
	}
	return out
}
