// SPDX-License-Identifier: MIT

// Package creds is the local credentials vault for provider env vars.
//
// Motivation (M1.o): the catalog pivot (M1.f–M1.n) made *providers* a
// catalog refresh away. Credentials were still shell env vars, which
// doesn't scale past a couple of providers — operators with 10+ keys
// don't want them all in rc files. The vault is a flat JSON file
// (`~/.agezt/creds.json`, 0600 perms) holding env-var-name → value
// pairs that the daemon's cred resolver chains with `os.Getenv`.
//
// Scope (M1.o):
//
//   - Plain-JSON storage. No encryption, no OS-keychain integration.
//     The vault file inherits the same 0600 perms as the rest of
//     `~/.agezt/` and lives next to the journal.
//   - agt writes; daemon reads on startup. Re-export to pick up
//     vault changes (matches catalog-reload UX from M1.f).
//   - Lookup precedence (in `ChainLookup`): vault first, then env.
//     This lets operators temporarily override a vaulted key by
//     `export`-ing in a session without rewriting the vault.
//
// Out of scope (deferred):
//
//   - At-rest encryption (M1.o.x — likely via the OS keychain on
//     macOS/Linux/Windows once we add a small platform-specific
//     dep).
//   - Hot reload by the daemon. Today the daemon snapshot is
//     captured on Open; SIGHUP-driven reload lands when the
//     credentials-rotation UX is fleshed out.
//   - Per-provider scoping. Env-var names are global (OPENAI_API_KEY
//     means the same thing wherever); a flat map is the right shape.
package creds
