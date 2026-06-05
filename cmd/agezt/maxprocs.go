// SPDX-License-Identifier: MIT

package main

import (
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// SPEC-11 §4 ("resource limits … don't hot-loop a Pi"): inside a CPU-quota
// cgroup (a container started with `--cpus=N`, a constrained VPS) the Go runtime
// still defaults GOMAXPROCS to the number of HOST cores — it is not cgroup-aware
// — so a 1-CPU deployment spins up NumCPU() Ps and over-schedules against a
// fraction of a core. uber-go/automaxprocs fixes this but is a dependency; the
// helpers below are the stdlib-only equivalent for the common cgroup v2
// (cpu.max) and v1 (cpu.cfs_quota_us / cpu.cfs_period_us) layouts.

// cgroupCPUQuota returns the fractional CPU quota (e.g. 1.5 = 150% of one core)
// from cgroup v2 first, then v1, and false when there is no finite quota (bare
// metal, or cgroup v2 "max"). The file reader is injected so this is testable on
// any OS without a real cgroup.
func cgroupCPUQuota(readFile func(string) ([]byte, error)) (float64, bool) {
	// cgroup v2: "<quota> <period>" in microseconds, or "max <period>".
	if b, err := readFile("/sys/fs/cgroup/cpu.max"); err == nil {
		f := strings.Fields(string(b))
		if len(f) == 2 && f[0] != "max" {
			q, e1 := strconv.ParseInt(f[0], 10, 64)
			p, e2 := strconv.ParseInt(f[1], 10, 64)
			if e1 == nil && e2 == nil && q > 0 && p > 0 {
				return float64(q) / float64(p), true
			}
		}
		return 0, false // "max" or unparseable → no finite quota
	}
	// cgroup v1: separate quota/period files.
	qb, e1 := readFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	pb, e2 := readFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if e1 == nil && e2 == nil {
		q, ea := strconv.ParseInt(strings.TrimSpace(string(qb)), 10, 64)
		p, eb := strconv.ParseInt(strings.TrimSpace(string(pb)), 10, 64)
		if ea == nil && eb == nil && q > 0 && p > 0 {
			return float64(q) / float64(p), true
		}
	}
	return 0, false
}

// cgroupMaxProcs computes the GOMAXPROCS the daemon should use given a cgroup CPU
// quota, host core count, and the GOMAXPROCS env value — or 0 to mean "leave the
// Go runtime default untouched". Pure (no globals, no OS calls) so it is fully
// unit-testable. It only ever LOWERS toward the quota, never below 1 or above the
// host core count, and defers entirely to an explicit GOMAXPROCS env. The quota
// is rounded UP (ceil) so a fractional cap isn't throttled to a smaller integer.
func cgroupMaxProcs(readFile func(string) ([]byte, error), numCPU int, gomaxprocsEnv string) (int, string) {
	if strings.TrimSpace(gomaxprocsEnv) != "" {
		return 0, "" // explicit GOMAXPROCS wins; don't second-guess the operator
	}
	quota, ok := cgroupCPUQuota(readFile)
	if !ok {
		return 0, "" // no finite CPU quota → the host-core default is correct
	}
	n := max(int(math.Ceil(quota)), 1)
	if numCPU > 0 && n >= numCPU {
		return 0, "" // quota ≥ host cores → default already correct, no change
	}
	return n, "cgroup CPU quota ≈ " + strconv.FormatFloat(quota, 'g', 3, 64) + " cores"
}

// applyAutoMaxProcs sets GOMAXPROCS from the cgroup CPU quota when running on
// Linux without an explicit GOMAXPROCS. Returns a one-line banner note describing
// the change, or "" when nothing was changed. On non-Linux the cgroup files are
// absent, so this is a clean no-op there too — but the GOOS guard makes the
// intent explicit and avoids the file probes entirely off Linux.
func applyAutoMaxProcs() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	n, reason := cgroupMaxProcs(os.ReadFile, runtime.NumCPU(), os.Getenv("GOMAXPROCS"))
	if n <= 0 {
		return ""
	}
	prev := runtime.GOMAXPROCS(n)
	return "GOMAXPROCS " + strconv.Itoa(prev) + " → " + strconv.Itoa(n) + " (" + reason + ")"
}
