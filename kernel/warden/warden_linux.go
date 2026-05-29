// SPDX-License-Identifier: MIT

//go:build linux

package warden

// Linux-specific warden hardening (M1.d). Applied when the
// requested profile is ProfileNamespace (or stronger) AND the host
// is Linux. Two layers:
//
//   1. **Pre-Start**: `SysProcAttr.Setpgid = true` so the child
//      starts in its own process group. We also override
//      `cmd.Cancel` to send SIGKILL to `-pgid` so the kill-on-
//      timeout path sweeps grandchildren a misbehaving tool spawned.
//      Without this, a tool that fork-execs a long-running helper
//      leaves the helper orphaned when the wrapper is killed.
//
//   2. **Post-Start (best-effort)**: prlimit(2) calls for
//      CPU / address space / open files / file size. Best-effort
//      because the child can do some allocation between Start and
//      the prlimit call — operators who need hard guarantees
//      should use ProfileContainer (M2+). For typical accidental-
//      runaway protection (a tool that mis-loops and tries to
//      allocate 40 GiB or open 10k files), this is enough.
//
// **What this is NOT.** No namespaces (CLONE_NEWUSER / CLONE_NEWNS
// / CLONE_NEWPID), no seccomp BPF, no cgroup v2. Those would be
// full M1.d.1 / M1.d.2 follow-ups. ProfileNamespace's "effective"
// profile under this implementation is still ProfileNamespace —
// the rlimit confinement IS the v1 namespace-equivalent layer,
// and EffectiveProfile reflects that. Future hardening adds
// strictly more — operators relying on ProfileNamespace today get
// upgraded confinement for free when those land.
//
// **Why Syscall6 + unsafe.** Stdlib's `syscall` package never
// exposed a `Prlimit` wrapper (it lives in golang.org/x/sys/unix,
// which agezt doesn't depend on per the lean-deps policy).
// SYS_PRLIMIT64 is exported per-arch and the Rlimit struct is
// already defined; calling the syscall directly with one
// unsafe.Pointer cast is the minimum-blast-radius way to bridge
// the gap. The unsafe block is confined to one helper.

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// resolveEffectiveProfile is the Linux-specific profile resolution
// for M1.d: ProfileNamespace is honored (engages setpgid + rlimits).
// Stronger profiles (Container / MicroVM) still need plugin
// backends not yet shipped, so they downgrade to Namespace as the
// next-best available layer.
func resolveEffectiveProfile(p Profile) Profile {
	switch p {
	case ProfileNone:
		return ProfileNone
	case ProfileNamespace:
		return ProfileNamespace
	case ProfileContainer, ProfileMicroVM:
		return ProfileNamespace
	}
	return ProfileNone
}

// configurePlatformAttrs sets cmd.SysProcAttr for process-group
// kill-on-timeout. Called before cmd.Start. effective is the
// post-downgrade profile — we only harden for ProfileNamespace
// or stronger; ProfileNone matches the cross-platform behavior.
func configurePlatformAttrs(cmd *exec.Cmd, effective Profile) {
	if effective == ProfileNone {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID targets the process group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// applyPlatformLimits is called immediately after cmd.Start.
// Each non-zero Limits.* field translates to a prlimit syscall.
// Failures are surfaced as warden.limit events but never abort
// the run — the wall-clock Timeout still bounds the worst case.
func applyPlatformLimits(cmd *exec.Cmd, spec Spec, effective Profile, e *engine) {
	if effective == ProfileNone || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid

	apply := func(resource int, name string, value uint64) {
		if err := prlimitSet(pid, resource, value); err != nil {
			e.publishLimitExceeded(spec, "prlimit_"+name+"_setfailed", err.Error())
		}
	}

	if spec.Limits.CPUSeconds > 0 {
		apply(syscall.RLIMIT_CPU, "cpu", uint64(spec.Limits.CPUSeconds))
	}
	if spec.Limits.AddressSpaceBytes > 0 {
		apply(syscall.RLIMIT_AS, "as", spec.Limits.AddressSpaceBytes)
	}
	if spec.Limits.MaxOpenFiles > 0 {
		apply(syscall.RLIMIT_NOFILE, "nofile", spec.Limits.MaxOpenFiles)
	}
	if spec.Limits.MaxFileSizeBytes > 0 {
		apply(syscall.RLIMIT_FSIZE, "fsize", spec.Limits.MaxFileSizeBytes)
	}
}

// prlimitSet wraps the prlimit64(2) syscall. Both soft and hard
// limits are set to value (single-value semantics match how every
// other agezt cap works — operators don't separately configure
// "soft" vs "hard"). Returns the errno-as-error from the kernel
// or nil on success.
func prlimitSet(pid, resource int, value uint64) error {
	lim := syscall.Rlimit{Cur: value, Max: value}
	_, _, errno := syscall.Syscall6(
		syscall.SYS_PRLIMIT64,
		uintptr(pid),
		uintptr(resource),
		uintptr(unsafe.Pointer(&lim)),
		0, // old_limit = NULL — we don't need the pre-existing value
		0,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
