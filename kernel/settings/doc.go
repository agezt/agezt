// SPDX-License-Identifier: MIT

// Package settings is the file-backed config store behind the Config Center
// (M693): the NON-SECRET, operator-editable settings the daemon would otherwise
// only read from environment variables. It complements the credentials vault
// (kernel/creds) — secrets go there; everything else lives here.
//
// Shape: `<baseDir>/config.json`, 0600, atomically written. Internally the data
// is keyed by ACCOUNT so a future multi-account dimension nests cleanly; today a
// single "_default" account is exposed through the account-less accessors. The
// keys are the exact `AGEZT_*` env-var names, so the daemon can inject these into
// the process environment at startup and the existing ~170 `os.Getenv` consumers
// read them unchanged — no rewrite of config plumbing.
package settings
