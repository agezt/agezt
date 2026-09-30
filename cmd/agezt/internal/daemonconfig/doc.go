// SPDX-License-Identifier: MIT

// Package daemonconfig parses the daemon's AGEZT_* boot environment into one
// typed Config (Phase 2.5). It replaces the ~450 lines of inline env parsing
// that lived in runDaemon, so every parse shape, default, warning string, and
// fatal-vs-warn decision is unit-testable without booting a daemon.
//
// Load MUST be called after injectConfig has bridged the Config Center store +
// vault into the process environment (the reads here see injected values, same
// as the inline code they replace). Reads that must NOT go through Load stay in
// runDaemon:
//
//   - AGEZT_FORCE_START — read before injectConfig runs (single-instance guard).
//   - AGEZT_COUNCIL_MEMBERS, AGEZT_DRAIN_TIMEOUT, and the tenant lazy-open's
//     AGEZT_EDICT_DURABLE — read live at call time (the Config Center
//     live-applies edits via os.Setenv, so a boot-time capture would regress
//     restart-free reconfiguration).
//   - channel-manifest RequiredEnv/AllowlistEnv names — dynamic, not AGEZT_*
//     statics.
//   - AGEZT_WEB_ADDR / AGEZT_REST_ADDR / AGEZT_API_ADDR — owned by their
//     buildWebUI/buildRESTAPI/buildOpenAIAPI helpers.
//
// Semantics contract: Load returns an error ONLY for the values that were a
// hard startup failure in the inline code (malformed durations/integers on the
// run-loop caps, malformed USD amounts, malformed deny rules, malformed tenant
// quotas when multi-tenancy is on). Everything else warns to the writer with
// the exact historical message and falls back to its default. Error strings
// carry the env name ("AGEZT_X: want ..."), so runDaemon's
// `fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)` reproduces the
// historical output byte-for-byte.
package daemonconfig
