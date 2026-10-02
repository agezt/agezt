// SPDX-License-Identifier: MIT

// Package toolbox is the host CLI-tool inventory + installer (M956). It answers
// "what command-line tools are on this machine, which are missing, and which are
// out of date" and resolves a per-OS package-manager command to install a
// missing one — so the operator can provision the agent's host from the web UI
// instead of hand-running winget/brew/apt.
//
// Detection is read-only (exec.LookPath + a bounded `--version` probe).
// Installation runs the real package manager at the HOST level (no isolation —
// an installer must be able to change the system, which is the opposite of what
// warden's sandbox provides), so it is gated by the authed control plane and
// every install is journaled by the caller.
package toolbox
