// SPDX-License-Identifier: MIT

// Package format hosts the pure formatting helpers used across the agt
// CLI's command surface. They were lifted verbatim from budget.go,
// doctor.go, main.go, runs.go, status.go, agent.go and token.go so the
// call sites can stay readable while the bodies live behind a stable,
// unit-tested API.
//
// Rules of the road:
//
//   - No I/O, no globals, no daemon state. Everything is a pure
//     function from primitives to strings.
//   - Render-only — do not couple to the wire protocol, brand strings,
//     or i18n. The agt CLI is single-locale by design (B0 spirit).
//   - DiskWarnPct is the only exported constant. It is a tunable used
//     by `agt doctor`'s disk-pressure check.
package format
