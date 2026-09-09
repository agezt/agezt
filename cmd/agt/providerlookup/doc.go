// SPDX-License-Identifier: MIT

// Package providerlookup builds the credential lookup chain the
// CLI uses to ask "does this provider have credentials?". It is
// the missing piece between the bare-name vault entries the
// legacy code wrote and the scoped `provider:<id>:<env>` entries
// the M700 keyring introduced — a bare key is intentionally
// GLOBAL for an env name that only ONE provider declares, but
// when two providers share an env name (e.g. both an
// `openai-compatible` adapter and the canonical `openai` adapter
// look at `OPENAI_API_KEY`), a bare key MUST NOT silently
// credential both.
//
// The package holds two builders:
//
//   - ScopedVaultLookup returns a function that consults a vault
//     but refuses to answer for a bare env name when two
//     providers claim it. The provider-scoped `provider:<id>:<env>`
//     name is always safe.
//   - CredentialLookup composes ScopedVaultLookup with
//     os.Getenv — the real process env remains the universal
//     fallback for shared env names (process env is global by
//     OS contract; only the vault, which we control, gets the
//     per-provider scoping).
//
// Why this lives in its own package. Both functions are pure —
// no I/O, no control-plane call, no state. They used to live in
// cmd/agt/provider_lookup.go next to the `provider` command
// itself, but the command depended on the helpers and the
// helpers were used only by unit tests in the same file. Moving
// them to their own package broke no callers and gave them a
// place to grow: any future "how do we know provider X has a
// credential?" question lives here, not in package main.
package providerlookup
