// SPDX-License-Identifier: MIT

// This file is intentionally empty. Day 20b's per-store openers
// experiment (17 small wrapper functions) was rolled back because
// the kernel's store-opening pattern threads the bus (and several
// other post-open wirings) through Open() itself, so a thin
// per-store helper gains very little. Day 20c will revisit the
// shape if we want to extract the bus-wiring dance into a
// separate "post-open" hook. For now the Open() function in
// kernel/runtime/compose.go remains the single entry point.
package compose
