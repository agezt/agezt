// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"testing"
)

// fakeCgroup returns a readFile that serves the given path→content map and
// ErrNotExist for anything else.
func fakeCgroup(files map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if v, ok := files[p]; ok {
			return []byte(v), nil
		}
		return nil, os.ErrNotExist
	}
}

func TestCgroupMaxProcs(t *testing.T) {
	const (
		v2   = "/sys/fs/cgroup/cpu.max"
		v1q  = "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"
		v1p  = "/sys/fs/cgroup/cpu/cpu.cfs_period_us"
		host = 8
	)
	cases := []struct {
		name  string
		files map[string]string
		cpu   int
		env   string
		want  int
	}{
		{"v2 two cores", map[string]string{v2: "200000 100000"}, host, "", 2},
		{"v2 three cores", map[string]string{v2: "300000 100000"}, host, "", 3},
		{"v2 half core rounds up to 1", map[string]string{v2: "50000 100000"}, host, "", 1},
		{"v2 1.5 cores rounds up to 2", map[string]string{v2: "150000 100000"}, host, "", 2},
		{"v2 max means no limit", map[string]string{v2: "max 100000"}, host, "", 0},
		{"v1 three cores", map[string]string{v1q: "300000", v1p: "100000"}, host, "", 3},
		{"v1 negative quota (unset)", map[string]string{v1q: "-1", v1p: "100000"}, host, "", 0},
		{"explicit GOMAXPROCS wins", map[string]string{v2: "100000 100000"}, host, "4", 0},
		{"quota >= host cores → no change", map[string]string{v2: "800000 100000"}, 4, "", 0},
		{"quota above host clamped away", map[string]string{v2: "1600000 100000"}, 4, "", 0},
		{"no cgroup files", map[string]string{}, host, "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := cgroupMaxProcs(fakeCgroup(c.files), c.cpu, c.env)
			if got != c.want {
				t.Fatalf("cgroupMaxProcs = %d, want %d", got, c.want)
			}
			if got > 0 && reason == "" {
				t.Error("a non-zero result must carry a reason for the banner")
			}
			if got == 0 && reason != "" {
				t.Errorf("a no-change result must have an empty reason, got %q", reason)
			}
		})
	}
}

func TestCgroupCPUQuota_V2PrecedesV1(t *testing.T) {
	// When both layouts are present (a hybrid host), v2's cpu.max is authoritative.
	files := map[string]string{
		"/sys/fs/cgroup/cpu.max":               "200000 100000", // 2 cores
		"/sys/fs/cgroup/cpu/cpu.cfs_quota_us":  "700000",        // 7 cores (should be ignored)
		"/sys/fs/cgroup/cpu/cpu.cfs_period_us": "100000",
	}
	q, ok := cgroupCPUQuota(fakeCgroup(files))
	if !ok || q != 2.0 {
		t.Fatalf("cgroupCPUQuota = (%v, %v), want (2, true) — v2 must win over v1", q, ok)
	}
}
