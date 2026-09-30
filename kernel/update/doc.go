// SPDX-License-Identifier: MIT

// Package update implements Agezt's self-update mechanism.
//
// Design principles (from the Council of Elders deliberation):
//   - Drain-before-restart: the running daemon stops accepting new tasks,
//     waits for in-flight work to complete (bounded by a hard timeout),
//     then swaps the binary.
//   - Atomic binary swap: the new binary is written to a staging path,
//     validated (SHA256 checksum), and atomically renamed into place using
//     os.Rename — which is atomic on both POSIX and Windows.
//   - Fail-safe on error: if validation fails or the swap cannot complete,
//     the current binary is left untouched and the daemon does NOT
//     auto-restart. Human intervention is required to recover.
//   - State persistence: the kernel's journal and state store survive the
//     swap intact (they live under baseDir, not alongside the binary).
//
// Update check sources (configurable):
//   - GitHub Releases API: https://api.github.com/repos/<owner>/<repo>/releases/latest
//   - Custom endpoint: check.agezt.com (returns {version, sha256, url})
//
// The mechanism is triggered by:
//   - `agezt update` CLI subcommand
//   - A background periodic check (configurable interval)
//   - The REST API: POST /api/v1/update
package update
