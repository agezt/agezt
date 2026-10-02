// SPDX-License-Identifier: MIT

// Package shell is the in-process shell tool. It runs commands via the
// platform's default shell ("cmd /C" on Windows, "sh -c" elsewhere) and
// returns combined stdout+stderr to the model.
//
// Execution is delegated to the kernel/warden Engine so timeout,
// output truncation, exit code propagation, and audit events
// (warden.executed, warden.profile_downgraded, warden.limit_exceeded)
// are all handled in one place — including the future Linux
// namespace+cgroups isolation when that backend ships in M1.d.
//
// SECURITY NOTE (M1.c): the cross-platform Warden Engine runs commands
// with the kernel's full privileges (ProfileNone). Edict's trust ladder
// + hard-deny rules are still the only gate on what the shell tool may
// execute. A request for ProfileNamespace is honoured *as a request*
// and journaled as a downgrade so audits stay honest.
package shell
