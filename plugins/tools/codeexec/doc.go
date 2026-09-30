// SPDX-License-Identifier: MIT

// Package codeexec is the in-process code-execution tool: it lets the agent
// WRITE a real program (Python, JavaScript/Node, or TypeScript/Deno), RUN it,
// see the output, and ITERATE — the "build whatever's needed" primitive
// (M683). Compute, scraping, data wrangling, one-off scripts and multi-call
// projects all go through here.
//
// Execution is delegated to the kernel/warden Engine (the same path the shell
// tool uses) so timeout, output truncation, exit-code propagation, working-
// directory scoping, best-effort Linux resource limits, and audit events are
// handled in one place. On top of that this tool adds, for EVERY run:
//
//   - a per-call ephemeral scratch dir (or a named persistent project dir) under
//     <baseDir>/sandbox, so one run can't see or clobber another agent's work;
//   - a SCRUBBED environment — the daemon's secrets (API keys, provider creds,
//     the whole AGEZT_* namespace) are never forwarded into model-written code;
//   - for Deno, an OS-level filesystem jail confined to the work dir (real on
//     every platform, Windows included), with network granted by default;
//   - honest reporting of the EFFECTIVE isolation profile (Python/Node get the
//     warden's profile — real on Linux+namespace, workdir/env/limits-only
//     elsewhere; the result and events never overstate containment).
//
// SECURITY NOTE: running arbitrary code is a high-blast-radius capability. It is
// gated by the `code.exec` Edict capability and every run is journaled
// (code.executed + warden.exec) so the operator can see, govern, and revert.
package codeexec
