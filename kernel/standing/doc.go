// SPDX-License-Identifier: MIT

// Package standing implements the standing-order model and store (SPEC-16 §4).
// A standing order is a durable wake rule: a named, pausable event/cron trigger
// that wakes a governed plan, optionally as a roster agent, within an initiative
// ceiling. This package owns the durable record and its lifecycle (CRUD); the
// runner that fires triggers and drives the observe→salience→initiative→briefing
// pipeline is layered on top.
//
// The SPEC sketches the order as YAML authored in Flow Studio, but Agezt is
// stdlib-first (no YAML dependency, DECISIONS B0c), so the on-disk and wire form
// is JSON — the same declarative shape, a different serialisation.
package standing
