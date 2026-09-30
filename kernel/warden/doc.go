// SPDX-License-Identifier: MIT

// Package warden is the process-isolation layer (SPEC-06 §2; TASKS
// P1-WARD-01). It defines four named isolation profiles and a single
// Engine interface that tools (shell, browser, third-party plugins) use
// to run external work without caring about the underlying OS
// mechanism.
//
// Profiles (SPEC-06 §2) — the SPEC's intent, NOT a description of what
// this package implements today. What each one actually does is below.
//
//	ProfileNone        in-process / direct exec, no isolation
//	ProfileNamespace   Linux namespaces + cgroups + seccomp
//	ProfileContainer   OCI container (Docker / Podman)
//	ProfileMicroVM     lightweight VM (firecracker-class)
//
// # What ships today (RCE-002)
//
// Read this before scoping anything to a profile. The table above is a
// roadmap and was previously easy to mistake for a guarantee.
//
//   - **On Linux**, ProfileNamespace engages `setpgid` (so kill-on-timeout
//     sweeps grandchildren) plus best-effort prlimit(2) caps on CPU,
//     address space, open files and file size. There are **no namespaces
//     (CLONE_NEWUSER/NEWNS/NEWPID), no seccomp BPF and no cgroup v2** —
//     see the header of warden_linux.go, which is the authority. Container
//     and MicroVM downgrade to this.
//   - **On every other platform**, including Windows and macOS, ALL
//     profiles resolve to ProfileNone. Nothing is isolated. Windows job
//     objects and macOS sandbox-exec are not implemented.
//
// EffectiveProfile reports the resolution and every Run emits
// `warden.profile_downgraded`, so the honest answer is always available at
// runtime — but callers must ASK. Keying a credential bucket off the
// requested profile instead of the effective one is exactly how secrets
// scoped to the isolated tier ended up in an un-isolated child (RCE-001).
//
// Container/microvm backends remain M2+ optional plugins (SPEC-06 §2.2).
//
// What the cross-platform Engine already enforces today:
//
//   - **Timeout**: ctx-with-deadline + cmd.WaitDelay to bound orphaned
//     child IO.
//   - **Output truncation**: stdout/stderr capped at MaxOutputBytes.
//   - **Working directory** scoping.
//   - **Environment scrubbing**: child inherits only an allowlist.
//   - **Audit**: every Run emits a `warden.executed` event with
//     {profile_effective, profile_requested, exit_code, durations, bytes,
//     truncated, timed_out}.
package warden
