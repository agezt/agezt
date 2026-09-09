// SPDX-License-Identifier: MIT

// Package configcenter is the typed, audited, environment-aware
// configuration surface of the daemon. It owns the canonical
// schema (per-section, per-key), secret classification
// (what-must-never-appear-in-a-response), audit log, and the
// approval flow that gates sensitive value changes — the same
// trust ladder (DECISIONS F3) the Edict engine enforces for
// tool calls applies to config writes.
package configcenter
