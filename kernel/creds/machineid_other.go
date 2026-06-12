// SPDX-License-Identifier: MIT

//go:build !windows && !linux && !darwin

package creds

// machineID has no stable identity source on this platform; the vault stays
// plaintext-at-rest unless AGEZT_VAULT_PASSPHRASE is set.
func machineID() string { return "" }
