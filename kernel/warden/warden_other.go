// SPDX-License-Identifier: MIT

//go:build !linux

package warden

// Non-Linux stub for the M1.d hardening helpers. The cross-platform
// engine calls these unconditionally; on non-Linux they're no-ops
// and the existing "downgrade to ProfileNone with audit event"
// pattern (see publishDowngradeOnce) tells the operator nothing
// stronger is available.

import "os/exec"

// resolveEffectiveProfile on non-Linux: everything downgrades to
// ProfileNone (M1.c behavior preserved). Stronger profiles need
// per-OS backends not in scope: Windows job objects, macOS
// sandbox-exec, etc. The audit event from publishDowngradeOnce
// tells the operator nothing stronger is available.
func resolveEffectiveProfile(_ Profile) Profile { return ProfileNone }

func configurePlatformAttrs(_ *exec.Cmd, _ Profile)                 {}
func applyPlatformLimits(_ *exec.Cmd, _ Spec, _ Profile, _ *engine) {}
